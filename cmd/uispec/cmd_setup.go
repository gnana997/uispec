package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	var auto bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Detect AI agents and configure MCP server",
		Long:  "Scans for installed AI agents (Claude Code, Cursor, VS Code, etc.) and configures the UISpec MCP server integration.",
		Annotations: map[string]string{"group": "server"},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := setupOptions{auto: auto}
			executeSetup(os.Stdin, os.Stdout, opts)
			return nil
		},
	}

	cmd.Flags().BoolVar(&auto, "auto", false, "Configure all detected agents with defaults")

	return cmd
}
