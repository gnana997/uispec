package main

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Verbosity levels for CLI output.
type Verbosity int

const (
	VerbQuiet   Verbosity = -1
	VerbDefault Verbosity = 0
	VerbVerbose Verbosity = 1
)

// OutputConfig holds global output state.
type OutputConfig struct {
	Verbosity Verbosity
	NoColor   bool // from NO_COLOR env or --no-color flag
	IsTTY     bool // detected from os.Stderr
	IsJSON    bool // set per-command when --json is used
}

var outCfg OutputConfig

// initOutputConfig initializes the global output config.
func initOutputConfig(quiet, verbose, noColor bool) {
	outCfg.NoColor = noColor || os.Getenv("NO_COLOR") != ""
	outCfg.IsTTY = isTTY(os.Stderr)

	switch {
	case quiet:
		outCfg.Verbosity = VerbQuiet
	case verbose:
		outCfg.Verbosity = VerbVerbose
	default:
		outCfg.Verbosity = VerbDefault
	}

	// When NoColor is set, style functions return plain strings (checked inline).
}

// isTTY reports whether f is a terminal.
func isTTY(f *os.File) bool {
	if fi, err := f.Stat(); err == nil {
		return fi.Mode()&os.ModeCharDevice != 0
	}
	return false
}

// --- Semantic styles ---

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))  // gray
	headerStyle  = lipgloss.NewStyle().Bold(true)
	boldStyle    = lipgloss.NewStyle().Bold(true)
)

func styleSuccess(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return successStyle.Render(s)
}

func styleError(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return errorStyle.Render(s)
}

func styleWarning(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return warningStyle.Render(s)
}

func styleDim(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return dimStyle.Render(s)
}

func styleHeader(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return headerStyle.Render(s)
}

func styleBold(s string) string {
	if outCfg.NoColor || outCfg.IsJSON {
		return s
	}
	return boldStyle.Render(s)
}
