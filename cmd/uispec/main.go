package main

import (
	"fmt"
	"os"
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
		fmt.Println("uispec serve — not yet implemented")
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
