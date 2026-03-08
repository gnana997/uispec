package scanner

import (
	"fmt"
	"io"
	"os"
)

// Progress writes scan phase status to stderr.
// On a TTY it overwrites the current line; otherwise it appends newlines.
type Progress struct {
	w     io.Writer
	isTTY bool
}

// NewProgress creates a Progress that writes to stderr.
// Pass false to disable (no-op progress).
func NewProgress(enabled bool) *Progress {
	if !enabled {
		return &Progress{}
	}
	isTTY := isTTYWriter(os.Stderr)
	return &Progress{w: os.Stderr, isTTY: isTTY}
}

// Phase writes a status line for the current scan phase.
func (p *Progress) Phase(name string, detail string) {
	if p == nil || p.w == nil {
		return
	}
	var line string
	if detail != "" {
		line = fmt.Sprintf("[scan] %s: %s", name, detail)
	} else {
		line = fmt.Sprintf("[scan] %s...", name)
	}
	if p.isTTY {
		fmt.Fprintf(p.w, "\r%-60s", line)
	} else {
		fmt.Fprintln(p.w, line)
	}
}

// Done clears the progress line on TTY.
func (p *Progress) Done() {
	if p != nil && p.w != nil && p.isTTY {
		// Clear the line and move cursor back.
		fmt.Fprintf(p.w, "\r%-60s\r", "")
	}
}

// isTTYWriter reports whether w is a terminal.
func isTTYWriter(w *os.File) bool {
	if fi, err := w.Stat(); err == nil {
		return fi.Mode()&os.ModeCharDevice != 0
	}
	return false
}
