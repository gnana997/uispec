package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/gnana997/uispec/pkg/mcplog"
	mcpserver "github.com/gnana997/uispec/pkg/mcp"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/validator"
)

func newServeCmd() *cobra.Command {
	var catalogFlag, logFilePath string
	var enableLog bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server",
		Long:  "Starts the Model Context Protocol server on stdio. AI agents connect to this to query the component catalog.",
		Annotations: map[string]string{"group": "server"},
		RunE: func(cmd *cobra.Command, args []string) error {
			logFile := logFilePath
			if enableLog && logFile == "" {
				logFile = ".uispec/logs/mcp.jsonl"
			}

			catalogPath := resolveCatalogPath(catalogFlag)
			qs, err := loadCatalog(catalogPath)
			if err != nil {
				return err
			}

			pm := parser.NewParserManager(nil)
			defer func() { _ = pm.Close() }()
			v := validator.NewValidator(qs.Catalog, qs.Index, pm)

			var logger *mcplog.Logger
			if logFile != "" {
				logger, err = mcplog.NewLogger(logFile)
				if err != nil {
					return fmt.Errorf("failed to open log file: %w", err)
				}
				defer func() { _ = logger.Close() }()
			}

			srv := mcpserver.NewServer(qs, v, logger)
			defer func() { _ = srv.Close() }()

			if err := srv.ServeStdio(); err != nil {
				// Write to stderr only — stdout is MCP protocol.
				fmt.Fprintf(os.Stderr, "server error: %v\n", err)
				return &ExitError{Code: 1, Err: err}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&catalogFlag, "catalog", "", "Path to component catalog")
	cmd.Flags().BoolVar(&enableLog, "log", false, "Log MCP calls to .uispec/logs/mcp.jsonl")
	cmd.Flags().StringVar(&logFilePath, "log-file", "", "Log MCP calls to a custom path")

	return cmd
}
