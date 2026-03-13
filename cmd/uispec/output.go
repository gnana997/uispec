package main

import (
	"fmt"
	"os"
	"strings"

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

// --- Structured output helpers ---

const phaseLineWidth = 50 // total width for phase description + timing

// renderPhaseLine formats a completed phase line with right-aligned timing.
// Example: "  ✓ Discovered 102 files                    12ms"
func renderPhaseLine(desc string, timeMs int64) string {
	check := styleSuccess("✓")
	timing := styleDim(fmt.Sprintf("%dms", timeMs))
	timingRaw := fmt.Sprintf("%dms", timeMs)

	// Pad description to right-align timing.
	pad := phaseLineWidth - len(desc) - len(timingRaw)
	if pad < 2 {
		pad = 2
	}
	return fmt.Sprintf("  %s %s%s%s", check, desc, strings.Repeat(" ", pad), timing)
}

// NextStep describes a suggested follow-up command.
type NextStep struct {
	Command     string
	Description string
}

// renderNextSteps prints a "Next steps" block with commands and descriptions.
func renderNextSteps(steps []NextStep) {
	if len(steps) == 0 || outCfg.Verbosity == VerbQuiet {
		return
	}

	fmt.Printf("\n  %s\n", styleHeader("Next steps"))

	// Find max command width for alignment.
	maxCmd := 0
	for _, s := range steps {
		if len(s.Command) > maxCmd {
			maxCmd = len(s.Command)
		}
	}

	for _, s := range steps {
		fmt.Printf("    %-*s  %s\n", maxCmd, styleBold(s.Command), styleDim(s.Description))
	}
}

// renderError formats a styled error with title, detail, and optional hint.
func renderError(title, detail, hint string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %s %s\n", styleError("✗"), styleBold(title)))
	if detail != "" {
		sb.WriteString(fmt.Sprintf("    %s\n", detail))
	}
	if hint != "" {
		sb.WriteString(fmt.Sprintf("\n    %s %s\n", styleDim("hint:"), styleDim(hint)))
	}
	return sb.String()
}

// hintForError returns a helpful hint for common error messages.
func hintForError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no component files found"):
		return "Check the directory path and ensure it contains .tsx/.jsx files"
	case strings.Contains(msg, "not found in catalog"):
		return "Run 'uispec inspect' without arguments to list available components"
	case strings.Contains(msg, "already exists"):
		return "Use --force to overwrite the existing configuration"
	case strings.Contains(msg, "typescript not found") || strings.Contains(msg, "Cannot find module 'typescript'"):
		return "Install typescript in your project: npm install typescript"
	case strings.Contains(msg, "duplicate sub-component"):
		return "This usually means the same component is exported from multiple files"
	case strings.Contains(msg, "no node or bun runtime"):
		return "Install Node.js (https://nodejs.org) for enrichment support"
	}
	return ""
}
