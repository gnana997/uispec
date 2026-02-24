package parser

import (
	"fmt"
	"log/slog"
	"sync"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// parserPool manages a pool of tree-sitter parsers for concurrent access.
//
// Design:
// - Channel-based pooling for thread-safe acquire/release
// - Lazy parser creation up to maxSize
// - All parsers use the same language grammar
// - Parsers are created on-demand as pool grows
//
// Thread Safety:
// - Channel operations are inherently thread-safe
// - Mutex protects parser creation and stats
type parserPool struct {
	// pool is a buffered channel storing available parsers
	pool chan *ts.Parser

	// langPtr is the tree-sitter language pointer for this pool
	langPtr unsafe.Pointer

	// lang is the language enum (for logging)
	lang Language

	// isTSX indicates if this is a TSX pool (only relevant for TypeScript)
	isTSX bool

	// maxSize is the maximum number of parsers in the pool
	maxSize int

	// mutex protects created count and parser creation
	mutex sync.Mutex

	// created tracks how many parsers have been created
	created int

	// logger for structured logging
	logger *slog.Logger
}

// newParserPool creates a new parser pool for a specific language.
//
// Parameters:
// - lang: The language enum
// - langPtr: The tree-sitter language pointer
// - isTSX: Whether this is a TSX pool (only for TypeScript)
// - maxSize: Maximum number of parsers to create
// - logger: Structured logger
func newParserPool(lang Language, langPtr unsafe.Pointer, isTSX bool, maxSize int, logger *slog.Logger) *parserPool {
	return &parserPool{
		pool:    make(chan *ts.Parser, maxSize),
		langPtr: langPtr,
		lang:    lang,
		isTSX:   isTSX,
		maxSize: maxSize,
		created: 0,
		logger:  logger,
	}
}

// acquire returns a parser from the pool, creating one if needed.
//
// Thread Safety:
// - Safe for concurrent use
// - Creates parsers lazily up to maxSize
// - Blocks if all parsers are in use and maxSize is reached
func (p *parserPool) acquire() (*ts.Parser, error) {
	select {
	case parser := <-p.pool:
		// Got a parser from the pool
		return parser, nil
	default:
		// Pool is empty - try to create a new parser
		return p.createParserIfNeeded()
	}
}

// createParserIfNeeded creates a new parser if we haven't reached maxSize.
// If maxSize is reached, it blocks waiting for a parser to be released.
func (p *parserPool) createParserIfNeeded() (*ts.Parser, error) {
	p.mutex.Lock()

	// Check if we can create a new parser
	if p.created < p.maxSize {
		// Create new parser
		parser := ts.NewParser()
		if parser == nil {
			p.mutex.Unlock()
			return nil, fmt.Errorf("failed to create parser")
		}

		// Set language
		tsLang := ts.NewLanguage(p.langPtr)
		if err := parser.SetLanguage(tsLang); err != nil {
			parser.Close()
			p.mutex.Unlock()
			return nil, fmt.Errorf("failed to set language: %w", err)
		}

		p.created++
		p.logger.Debug("created parser in pool",
			"language", p.lang.String(),
			"isTSX", p.isTSX,
			"pool_size", p.created)

		p.mutex.Unlock()
		return parser, nil
	}

	// Max size reached - wait for a parser to be released
	p.mutex.Unlock()
	parser := <-p.pool
	return parser, nil
}

// release returns a parser to the pool for reuse.
//
// Thread Safety:
// - Safe for concurrent use
// - Non-blocking (channel is buffered)
func (p *parserPool) release(parser *ts.Parser) {
	if parser == nil {
		return
	}

	select {
	case p.pool <- parser:
		// Successfully returned to pool
	default:
		// Pool is full (shouldn't happen with proper usage)
		// Close the parser to avoid leak
		parser.Close()
		p.logger.Warn("parser pool full, closing excess parser",
			"language", p.lang.String())
	}
}

// close releases all parsers in the pool.
//
// After calling close, the pool cannot be used.
func (p *parserPool) close() {
	close(p.pool)

	// Drain and close all parsers in the pool
	count := 0
	for parser := range p.pool {
		if parser != nil {
			parser.Close()
			count++
		}
	}

	p.logger.Debug("closed parser pool",
		"language", p.lang.String(),
		"isTSX", p.isTSX,
		"parsers_closed", count)
}

// getCreatedCount returns the number of parsers created in this pool.
func (p *parserPool) getCreatedCount() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.created
}
