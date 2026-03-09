package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// knownCatalog describes a pre-built catalog available from GitHub releases.
type knownCatalog struct {
	Name string
	Desc string
	File string
}

var knownCatalogs = []knownCatalog{
	{"shadcn", "shadcn/ui component library", "shadcn-catalog.json"},
	{"radix", "Radix UI primitives", "radix-catalog.json"},
}

const (
	catalogGitHubOwner = "gnana997"
	catalogGitHubRepo  = "uispec"
)

// userCatalogDir returns ~/.uispec/catalogs/, creating it if needed.
func userCatalogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".uispec", "catalogs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create catalog directory: %w", err)
	}
	return dir, nil
}

func runCatalog(args []string) {
	if len(args) == 0 {
		printCatalogUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "pull":
		runCatalogPull(args[1:])
	case "list":
		runCatalogList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown catalog subcommand: %s\n", args[0])
		printCatalogUsage()
		os.Exit(1)
	}
}

func printCatalogUsage() {
	fmt.Println("Usage: uispec catalog <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  list                       List available and installed catalogs")
	fmt.Println("  pull <name> [--version v]   Download a pre-built catalog")
	fmt.Println()
	fmt.Println("Available catalogs:")
	for _, c := range knownCatalogs {
		fmt.Printf("  %-10s %s\n", c.Name, c.Desc)
	}
}

func runCatalogPull(args []string) {
	var name, ver string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version":
			if i+1 < len(args) {
				i++
				ver = args[i]
			}
		default:
			if !strings.HasPrefix(args[i], "--") && name == "" {
				name = args[i]
			}
		}
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "usage: uispec catalog pull <name> [--version <tag>]")
		os.Exit(1)
	}

	// Look up catalog.
	var cat *knownCatalog
	for i := range knownCatalogs {
		if knownCatalogs[i].Name == name {
			cat = &knownCatalogs[i]
			break
		}
	}
	if cat == nil {
		fmt.Fprintf(os.Stderr, "unknown catalog: %q\n", name)
		fmt.Fprintln(os.Stderr, "Available catalogs:")
		for _, c := range knownCatalogs {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Name)
		}
		os.Exit(1)
	}

	// Build download URL.
	var url string
	if ver == "" || ver == "latest" {
		url = fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s",
			catalogGitHubOwner, catalogGitHubRepo, cat.File)
	} else {
		url = fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
			catalogGitHubOwner, catalogGitHubRepo, ver, cat.File)
	}

	// Determine destination.
	dir, err := userCatalogDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	destPath := filepath.Join(dir, cat.File)

	fmt.Printf("Downloading %s catalog...\n", cat.Name)

	// Download.
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("User-Agent", "uispec/"+version)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "download failed: HTTP %d\n", resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			fmt.Fprintln(os.Stderr, "Catalog not found. It may not be available for this release yet.")
			if ver == "" {
				fmt.Fprintln(os.Stderr, "Try specifying a version: uispec catalog pull "+name+" --version v0.1.0")
			}
		}
		os.Exit(1)
	}

	// Read body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}

	// Validate JSON.
	if !json.Valid(body) {
		fmt.Fprintln(os.Stderr, "error: downloaded file is not valid JSON")
		os.Exit(1)
	}

	// Atomic write: temp file then rename.
	tmpFile, err := os.CreateTemp(dir, ".tmp-catalog-*.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(body); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "error writing catalog: %v\n", err)
		os.Exit(1)
	}
	_ = tmpFile.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sizeKB := len(body) / 1024
	fmt.Printf("Saved %s (%d KB) to %s\n", cat.File, sizeKB, destPath)
}

func runCatalogList(_ []string) {
	// Remote catalogs.
	fmt.Println("Available catalogs:")
	for _, c := range knownCatalogs {
		fmt.Printf("  %-10s %s\n", c.Name, c.Desc)
	}

	// Local catalogs.
	dir, err := userCatalogDir()
	if err != nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var locals []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			locals = append(locals, e)
		}
	}

	if len(locals) == 0 {
		fmt.Println("\nNo catalogs installed locally.")
		fmt.Println("Use 'uispec catalog pull <name>' to download one.")
		return
	}

	fmt.Printf("\nInstalled (%s):\n", dir)
	for _, e := range locals {
		info, err := e.Info()
		if err != nil {
			continue
		}
		sizeKB := info.Size() / 1024
		date := info.ModTime().Format("2006-01-02")
		fmt.Printf("  %-30s %4d KB    %s\n", e.Name(), sizeKB, date)
	}
}
