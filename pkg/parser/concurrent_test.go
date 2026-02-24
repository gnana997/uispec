package parser

import (
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentParsing tests that 100 goroutines can parse simultaneously
// without race conditions or deadlocks.
func TestConcurrentParsing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect errors
	errChan := make(chan error, numGoroutines)

	// Launch 100 goroutines that all parse TypeScript
	source := []byte("const x: number = 1;")
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			tree, err := manager.Parse(source, LanguageTypeScript, false)
			if err != nil {
				errChan <- err
				return
			}
			if tree == nil {
				errChan <- assert.AnError
				return
			}

			// Close the tree immediately
			tree.Close()
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "No errors should occur during concurrent parsing")

	// Verify stats
	stats := manager.GetStats()
	// With parser pool, we may create up to getDefaultPoolSize() parsers
	maxPoolSize := getDefaultPoolSize()
	assert.LessOrEqual(t, stats.ParsersCreated, maxPoolSize, "Should create at most %d parsers in pool", maxPoolSize)
	assert.GreaterOrEqual(t, stats.ParsersCreated, 1, "Should create at least 1 parser")
	assert.Equal(t, numGoroutines, stats.ParsesCalled, "Should have called Parse 100 times")
}

// TestConcurrentMultiLanguage tests concurrent parsing of different languages.
func TestConcurrentMultiLanguage(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	const goroutinesPerLanguage = 20
	languages := SupportedLanguages()
	numGoroutines := len(languages) * goroutinesPerLanguage

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines)

	// Launch goroutines for each language
	for _, lang := range languages {
		for i := 0; i < goroutinesPerLanguage; i++ {
			go func(l Language, id int) {
				defer wg.Done()

				source := []byte("const x = 1;")
				tree, err := manager.Parse(source, l, false)
				if err != nil {
					errChan <- err
					return
				}
				if tree == nil {
					errChan <- assert.AnError
					return
				}

				tree.Close()
			}(lang, i)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "No errors should occur during multi-language concurrent parsing")

	// Verify stats
	stats := manager.GetStats()
	// With parser pool, each language may create up to getDefaultPoolSize() parsers
	maxPoolSize := getDefaultPoolSize()
	maxParsers := len(languages) * maxPoolSize
	assert.LessOrEqual(t, stats.ParsersCreated, maxParsers, "Should create at most %d parsers", maxParsers)
	assert.GreaterOrEqual(t, stats.ParsersCreated, len(languages), "Should create at least one parser per language")
	assert.Equal(t, numGoroutines, stats.ParsesCalled, "Should have called Parse for all goroutines")
}

// TestConcurrentLazyInitialization tests that lazy initialization is thread-safe.
// Multiple goroutines try to trigger parser creation for the same language simultaneously.
func TestConcurrentLazyInitialization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines)

	// All goroutines start at the same time and try to parse the same language
	// This tests the double-checked locking pattern
	startBarrier := make(chan struct{})

	source := []byte("function test() { return 42; }")
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Wait for start signal
			<-startBarrier

			tree, err := manager.Parse(source, LanguageJavaScript, false)
			if err != nil {
				errChan <- err
				return
			}
			if tree == nil {
				errChan <- assert.AnError
				return
			}

			tree.Close()
		}(i)
	}

	// Signal all goroutines to start simultaneously
	close(startBarrier)

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "No errors should occur during concurrent lazy initialization")

	// Verify pool handles concurrent initialization correctly
	stats := manager.GetStats()
	// Multiple parsers may be created if goroutines run concurrently enough
	maxPoolSize := getDefaultPoolSize()
	assert.LessOrEqual(t, stats.ParsersCreated, maxPoolSize, "Should create at most %d parsers", maxPoolSize)
	assert.GreaterOrEqual(t, stats.ParsersCreated, 1, "Should create at least 1 parser")
	assert.Equal(t, numGoroutines, stats.ParsesCalled, "Should have called Parse 50 times")
}

// TestConcurrentTSXSwitch tests concurrent parsing with TypeScript/TSX grammar switching.
func TestConcurrentTSXSwitch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Both TS and TSX

	errChan := make(chan error, numGoroutines*2)

	tsSource := []byte("const x: number = 1;")
	tsxSource := []byte("const el = <div>Hello</div>;")

	// Half parse TypeScript, half parse TSX - interleaved
	for i := 0; i < numGoroutines; i++ {
		// TypeScript goroutine
		go func(id int) {
			defer wg.Done()

			tree, err := manager.Parse(tsSource, LanguageTypeScript, false)
			if err != nil {
				errChan <- err
				return
			}
			if tree == nil {
				errChan <- assert.AnError
				return
			}

			tree.Close()
		}(i)

		// TSX goroutine
		go func(id int) {
			defer wg.Done()

			tree, err := manager.Parse(tsxSource, LanguageTypeScript, true)
			if err != nil {
				errChan <- err
				return
			}
			if tree == nil {
				errChan <- assert.AnError
				return
			}

			tree.Close()
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "No errors should occur during TS/TSX concurrent parsing")
}

// TestConcurrentParseFile tests concurrent file parsing with auto-detection.
func TestConcurrentParseFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	testFiles := []struct {
		fileName string
		content  []byte
	}{
		{"test.ts", []byte("const x: number = 1;")},
		{"test.js", []byte("const x = 1;")},
	}

	const goroutinesPerFile = 20
	numGoroutines := len(testFiles) * goroutinesPerFile

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines)

	for _, tf := range testFiles {
		for i := 0; i < goroutinesPerFile; i++ {
			go func(fileName string, content []byte, id int) {
				defer wg.Done()

				tree, err := manager.ParseFile(content, fileName)
				if err != nil {
					errChan <- err
					return
				}
				if tree == nil {
					errChan <- assert.AnError
					return
				}

				tree.Close()
			}(tf.fileName, tf.content, i)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	assert.Empty(t, errors, "No errors should occur during concurrent ParseFile")
}

// TestRaceConditions tests for race conditions using Go's race detector.
// Run with: go test -race ./pkg/parser
func TestRaceConditions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Read and write operations

	// Goroutines performing Parse operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			lang := SupportedLanguages()[id%len(SupportedLanguages())]
			source := []byte("const x = 1;")

			tree, err := manager.Parse(source, lang, false)
			if err == nil && tree != nil {
				tree.Close()
			}
		}(i)
	}

	// Goroutines reading stats
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			_ = manager.GetStats()
		}(i)
	}

	wg.Wait()
}

// BenchmarkConcurrentParsing benchmarks concurrent parsing performance.
func BenchmarkConcurrentParsing(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := []byte("const x: number = 1;")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			tree, err := manager.Parse(source, LanguageTypeScript, false)
			if err != nil {
				b.Fatal(err)
			}
			tree.Close()
		}
	})
}

// BenchmarkSequentialParsing benchmarks sequential parsing performance.
func BenchmarkSequentialParsing(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager := NewParserManager(logger)
	defer manager.Close()

	source := []byte("const x: number = 1;")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := manager.Parse(source, LanguageTypeScript, false)
		if err != nil {
			b.Fatal(err)
		}
		tree.Close()
	}
}
