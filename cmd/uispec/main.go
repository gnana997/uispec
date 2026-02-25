package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gnana997/uispec/pkg/catalog"
	mcpserver "github.com/gnana997/uispec/pkg/mcp"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/validator"
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
		runValidate(os.Args[2:])
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
		pm := parser.NewParserManager(nil)
		defer pm.Close()
		v := validator.NewValidator(qs.Catalog, qs.Index, pm)
		srv := mcpserver.NewServer(qs, v)
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

func runValidate(args []string) {
	var filePath, catalogPath string
	autoFix := false
	asJSON := false

	catalogPath = "catalogs/shadcn/catalog.json"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--catalog":
			if i+1 < len(args) {
				i++
				catalogPath = args[i]
			}
		case "--fix":
			autoFix = true
		case "--json":
			asJSON = true
		default:
			if !strings.HasPrefix(args[i], "--") {
				filePath = args[i]
			}
		}
	}

	if filePath == "" {
		fmt.Fprintln(os.Stderr, "usage: uispec validate <file.tsx> [--catalog path] [--fix] [--json]")
		os.Exit(1)
	}

	code, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read file: %v\n", err)
		os.Exit(1)
	}

	// Resolve catalog path.
	if !filepath.IsAbs(catalogPath) {
		if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
			exe, _ := os.Executable()
			alt := filepath.Join(filepath.Dir(exe), catalogPath)
			if _, err := os.Stat(alt); err == nil {
				catalogPath = alt
			}
		}
	}

	qs, err := catalog.LoadAndQuery(catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load catalog: %v\n", err)
		os.Exit(1)
	}

	pm := parser.NewParserManager(nil)
	defer pm.Close()
	v := validator.NewValidator(qs.Catalog, qs.Index, pm)

	result := v.ValidatePage(string(code), autoFix)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		if !result.Valid {
			os.Exit(2)
		}
		return
	}

	// Human-readable output.
	if result.Valid {
		fmt.Printf("✓ %s — no violations\n", filePath)
	} else {
		fmt.Printf("✗ %s — %d violation(s)\n", filePath, len(result.Violations))
		for _, viol := range result.Violations {
			sev := strings.ToUpper(viol.Severity[:1]) + viol.Severity[1:]
			fmt.Printf("  [%s] line %d:%d  %s  (%s)\n", sev, viol.Line, viol.Column, viol.Message, viol.Rule)
			if viol.Suggestion != "" {
				fmt.Printf("         → %s\n", viol.Suggestion)
			}
		}
	}

	if autoFix && len(result.Fixes) > 0 {
		fmt.Printf("\n%d fix(es) applied — writing %s\n", len(result.Fixes), filePath)
		if err := os.WriteFile(filePath, []byte(result.FixedCode), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write fixed file: %v\n", err)
			os.Exit(1)
		}
	}

	if !result.Valid {
		os.Exit(2)
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
