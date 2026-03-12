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

	"github.com/spf13/cobra"
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

func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage pre-built catalogs",
		Long:  "List available catalogs and download pre-built component catalogs from GitHub releases.",
		Annotations: map[string]string{"group": "info"},
	}

	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogPullCmd())

	return cmd
}

func newCatalogListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available and installed catalogs",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Remote catalogs.
			fmt.Println(styleHeader("Available catalogs:"))
			for _, c := range knownCatalogs {
				fmt.Printf("  %-10s %s\n", styleBold(c.Name), c.Desc)
			}

			// Local catalogs.
			dir, err := userCatalogDir()
			if err != nil {
				return nil
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil
			}

			var locals []os.DirEntry
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					locals = append(locals, e)
				}
			}

			if len(locals) == 0 {
				fmt.Printf("\n%s\n", styleDim("No catalogs installed locally."))
				fmt.Printf("Use '%s' to download one.\n", styleBold("uispec catalog pull <name>"))
				return nil
			}

			fmt.Printf("\n%s %s\n", styleHeader("Installed"), styleDim("("+dir+")"))
			for _, e := range locals {
				info, err := e.Info()
				if err != nil {
					continue
				}
				sizeKB := info.Size() / 1024
				date := info.ModTime().Format("2006-01-02")
				fmt.Printf("  %-30s %4d KB    %s\n", e.Name(), sizeKB, styleDim(date))
			}
			return nil
		},
	}
}

func newCatalogPullCmd() *cobra.Command {
	var ver string

	cmd := &cobra.Command{
		Use:   "pull <name>",
		Short: "Download a pre-built catalog",
		Long:  "Downloads a pre-built component catalog from GitHub releases.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

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
				return fmt.Errorf("unknown catalog %q", name)
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
				return err
			}
			destPath := filepath.Join(dir, cat.File)

			if outCfg.Verbosity != VerbQuiet {
				fmt.Printf("Downloading %s catalog...\n", cat.Name)
			}

			// Download.
			client := &http.Client{Timeout: 60 * time.Second}
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return err
			}
			req.Header.Set("User-Agent", "uispec/"+version)

			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					msg := "Catalog not found. It may not be available for this release yet."
					if ver == "" {
						msg += "\nTry specifying a version: uispec catalog pull " + name + " --version v0.1.0"
					}
					return fmt.Errorf("download failed: HTTP %d\n%s", resp.StatusCode, msg)
				}
				return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
			}

			// Read body.
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}

			// Validate JSON.
			if !json.Valid(body) {
				return fmt.Errorf("downloaded file is not valid JSON")
			}

			// Atomic write: temp file then rename.
			tmpFile, err := os.CreateTemp(dir, ".tmp-catalog-*.json")
			if err != nil {
				return err
			}
			tmpPath := tmpFile.Name()

			if _, err := tmpFile.Write(body); err != nil {
				_ = tmpFile.Close()
				_ = os.Remove(tmpPath)
				return fmt.Errorf("error writing catalog: %w", err)
			}
			_ = tmpFile.Close()

			if err := os.Rename(tmpPath, destPath); err != nil {
				_ = os.Remove(tmpPath)
				return err
			}

			sizeKB := len(body) / 1024
			if outCfg.Verbosity == VerbQuiet {
				fmt.Println(destPath)
			} else {
				fmt.Printf("%s Saved %s (%d KB) to %s\n",
					styleSuccess("✓"), cat.File, sizeKB, destPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&ver, "version", "", "Release tag (e.g. v0.1.0)")

	return cmd
}
