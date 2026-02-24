package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gnana997/uispec/pkg/catalog"
	mcpserver "github.com/gnana997/uispec/pkg/mcp"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "init":
		fmt.Println("uispec init — not yet implemented")
	case "scan":
		fmt.Println("uispec scan — not yet implemented")
	case "validate":
		fmt.Println("uispec validate — not yet implemented")
	case "inspect":
		fmt.Println("uispec inspect — not yet implemented")
	case "serve":
		catalogPath := "catalogs/shadcn/catalog.json"
		// Check for --catalog flag.
		for i, arg := range os.Args[2:] {
			if arg == "--catalog" && i+1 < len(os.Args[2:])-1 {
				catalogPath = os.Args[2+i+2]
				break
			}
		}
		// Resolve relative to executable or working directory.
		if !filepath.IsAbs(catalogPath) {
			if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
				// Try relative to the executable.
				exe, _ := os.Executable()
				altPath := filepath.Join(filepath.Dir(exe), catalogPath)
				if _, err := os.Stat(altPath); err == nil {
					catalogPath = altPath
				}
			}
		}
		qs, err := catalog.LoadAndQuery(catalogPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to load catalog: %v\n", err)
			os.Exit(1)
		}
		srv := mcpserver.NewServer(qs)
		if err := srv.ServeStdio(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	case "watch":
		fmt.Println("uispec watch — not yet implemented")
	case "version":
		fmt.Printf("uispec %s\n", version)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: uispec <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init       Initialize a new uispec project")
	fmt.Println("  scan       Scan component library and generate catalog")
	fmt.Println("  validate   Validate code against catalog")
	fmt.Println("  inspect    Inspect a component's props and usage")
	fmt.Println("  serve      Start MCP server")
	fmt.Println("  watch      Watch for file changes")
	fmt.Println("  version    Print version")
	fmt.Println("  help       Show this help message")
}
