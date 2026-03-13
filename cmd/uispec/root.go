package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// ExitError signals a non-standard exit code (e.g. 2 for validation failure).
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }

var rootCmd = &cobra.Command{
	Use:   "uispec",
	Short: "Give AI agents deep knowledge of your component library",
	Long:  "UISpec scans your component library and generates catalogs that help AI agents write correct, idiomatic UI code.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute sets up and runs the root command.
func Execute(ver, cmt, dt string) {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", ver, cmt, dt)

	// Set custom version template to match previous output format.
	rootCmd.SetVersionTemplate("uispec {{.Version}}\n")

	// Persistent flags.
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Minimal output")
	rootCmd.PersistentFlags().Bool("verbose", false, "Detailed output with timing")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		quiet, _ := cmd.Flags().GetBool("quiet")
		verbose, _ := cmd.Flags().GetBool("verbose")
		noColor, _ := cmd.Flags().GetBool("no-color")

		if quiet && verbose {
			return fmt.Errorf("cannot use --quiet and --verbose together")
		}

		initOutputConfig(quiet, verbose, noColor)
		return nil
	}

	// Set custom grouped help template.
	cobra.AddTemplateFunc("groupedCommands", groupedCommandsFunc)
	rootCmd.SetUsageTemplate(usageTemplate)

	// Register commands.
	rootCmd.AddCommand(
		newInitCmd(),
		newScanCmd(),
		newValidateCmd(),
		newInspectCmd(),
		newServeCmd(),
		newSetupCmd(),
		newCatalogCmd(),
		newWatchCmd(),
		newVersionCmd(ver, cmt, dt),
	)

	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		if outCfg.IsJSON {
			fmt.Fprintln(os.Stderr, err)
		} else {
			fmt.Fprint(os.Stderr, renderError("Error", err.Error(), hintForError(err)))
		}
		os.Exit(1)
	}
}

// groupedCommandsFunc returns commands grouped by their "group" annotation.
func groupedCommandsFunc(cmds []*cobra.Command) string {
	groups := []struct {
		Title string
		Key   string
	}{
		{"Core Commands", "core"},
		{"Server Commands", "server"},
		{"Info Commands", "info"},
	}

	var sb strings.Builder
	for _, g := range groups {
		var matching []*cobra.Command
		for _, cmd := range cmds {
			if cmd.Annotations["group"] == g.Key && cmd.Name() != "help" {
				matching = append(matching, cmd)
			}
		}
		if len(matching) == 0 {
			continue
		}
		sb.WriteString(g.Title + ":\n")
		for _, cmd := range matching {
			sb.WriteString(fmt.Sprintf("  %-12s %s\n", cmd.Name(), cmd.Short))
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

var usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}

{{ groupedCommands .Commands}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

func init() {
	// Register the template function so it's available.
	cobra.AddTemplateFunc("groupedCommands", func(cmds []*cobra.Command) string {
		return groupedCommandsFunc(cmds)
	})
}
