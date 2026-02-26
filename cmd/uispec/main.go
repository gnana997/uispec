package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gnana997/uispec/catalogs"
	"github.com/gnana997/uispec/pkg/mcplog"
	mcpserver "github.com/gnana997/uispec/pkg/mcp"
	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/validator"
)

// Set via -ldflags at build time by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Support GNU-style --version and --help flags.
	if command == "--version" || command == "-v" {
		printVersion()
		return
	}
	if command == "--help" || command == "-h" {
		printUsage()
		return
	}

	switch command {
	case "init":
		runInit(os.Args[2:])
	case "scan":
		fmt.Println("uispec scan — not yet implemented")
	case "validate":
		runValidate(os.Args[2:])
	case "inspect":
		runInspect(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "watch":
		fmt.Println("uispec watch — not yet implemented")
	case "version":
		printVersion()
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("uispec %s (commit: %s, built: %s)\n", version, commit, date)
}

func runServe(args []string) {
	catalogFlag := ""
	logFile := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--catalog":
			if i+1 < len(args) {
				i++
				catalogFlag = args[i]
			}
		case "--log":
			logFile = ".uispec/logs/mcp.jsonl"
		case "--log-file":
			if i+1 < len(args) {
				i++
				logFile = args[i]
			}
		}
	}

	catalogPath := resolveCatalogPath(catalogFlag)
	qs, err := loadCatalog(catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	pm := parser.NewParserManager(nil)
	defer pm.Close()
	v := validator.NewValidator(qs.Catalog, qs.Index, pm)

	var logger *mcplog.Logger
	if logFile != "" {
		logger, err = mcplog.NewLogger(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open log file: %v\n", err)
			os.Exit(1)
		}
		defer logger.Close()
	}

	srv := mcpserver.NewServer(qs, v, logger)
	defer srv.Close()

	if err := srv.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func runValidate(args []string) {
	var filePath, catalogFlag string
	autoFix := false
	asJSON := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--catalog":
			if i+1 < len(args) {
				i++
				catalogFlag = args[i]
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

	catalogPath := resolveCatalogPath(catalogFlag)
	qs, err := loadCatalog(catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
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

func runInit(args []string) {
	preset := "shadcn" // default preset
	catalogFlag := ""
	force := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--preset":
			if i+1 < len(args) {
				i++
				preset = args[i]
			}
		case "--catalog":
			if i+1 < len(args) {
				i++
				catalogFlag = args[i]
			}
		case "--force":
			force = true
		}
	}

	if catalogFlag != "" {
		preset = "" // explicit --catalog overrides the default preset
	}
	if preset != "" && preset != "shadcn" {
		fmt.Fprintf(os.Stderr, "error: unknown preset %q (available: shadcn)\n", preset)
		os.Exit(1)
	}

	// Create .uispec/catalogs/ directory.
	if err := os.MkdirAll(".uispec/catalogs", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating .uispec/: %v\n", err)
		os.Exit(1)
	}

	// Guard against overwrite.
	if !force {
		if _, err := os.Stat(".uispec/config.yaml"); err == nil {
			fmt.Fprintln(os.Stderr, "error: .uispec/config.yaml already exists — use --force to overwrite")
			os.Exit(1)
		}
	}

	catalogPath := catalogFlag

	// Always write shadcn catalog unless a custom --catalog path was provided.
	if preset == "shadcn" {
		dest := ".uispec/catalogs/shadcn.json"
		if err := os.WriteFile(dest, catalogs.ShadcnJSON, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing preset catalog: %v\n", err)
			os.Exit(1)
		}
		catalogPath = dest
		fmt.Printf("wrote %s (%d bytes)\n", dest, len(catalogs.ShadcnJSON))
	}

	// Build and write config YAML.
	cfg := ProjectConfig{
		Version:     "1",
		Framework:   "react",
		CatalogPath: catalogPath,
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling config: %v\n", err)
		os.Exit(1)
	}
	header := "# UISpec project configuration\n# Generated by: uispec init\n"
	if err := os.WriteFile(".uispec/config.yaml", append([]byte(header), body...), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("created .uispec/config.yaml")
}

func runInspect(args []string) {
	componentName := ""
	catalogFlag := ""
	asJSON := false
	showExamples := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--catalog":
			if i+1 < len(args) {
				i++
				catalogFlag = args[i]
			}
		case "--json":
			asJSON = true
		case "--examples":
			showExamples = true
		default:
			if !strings.HasPrefix(args[i], "--") {
				componentName = args[i]
			}
		}
	}

	if componentName == "" {
		fmt.Fprintln(os.Stderr, "usage: uispec inspect <ComponentName> [--catalog path] [--json] [--examples]")
		os.Exit(1)
	}

	catalogPath := resolveCatalogPath(catalogFlag)
	qs, err := loadCatalog(catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	comp, found := qs.GetComponent(componentName)
	if !found {
		fmt.Fprintf(os.Stderr, "error: component %q not found in catalog\n", componentName)
		os.Exit(1)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(comp)
		return
	}

	// Is this a sub-component lookup?
	_, isTopLevel := qs.Index.ComponentByName[componentName]
	isSubComp := !isTopLevel

	printComponentHuman(comp, isSubComp, componentName, showExamples)
}

func printUsage() {
	fmt.Println("Usage: uispec <command>")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init       Initialize a new uispec project (shadcn preset by default)")
	fmt.Println("             --preset shadcn   Use bundled shadcn catalog (default)")
	fmt.Println("             --catalog <path>  Use a custom catalog path instead")
	fmt.Println("             --force           Overwrite existing config")
	fmt.Println("  inspect    Inspect a component's props and usage")
	fmt.Println("             <Component> [--catalog path] [--json] [--examples]")
	fmt.Println("  scan       Scan component library and generate catalog")
	fmt.Println("  validate   Validate code against catalog")
	fmt.Println("             <file.tsx> [--catalog path] [--fix] [--json]")
	fmt.Println("  serve      Start MCP server")
	fmt.Println("             --catalog <path>      Use a custom catalog path")
	fmt.Println("             --log                 Log MCP calls to .uispec/logs/mcp.jsonl")
	fmt.Println("             --log-file <path>     Log MCP calls to a custom path")
	fmt.Println("  watch      Watch for file changes")
	fmt.Println("  version    Print version")
	fmt.Println("  help       Show this help message")
}
