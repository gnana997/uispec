package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gnana997/uispec/pkg/scanner"
)

func newScanCmd() *cobra.Command {
	var output, name, importPrefix, categoriesStr string
	var noEnrich, initConfig bool

	cmd := &cobra.Command{
		Use:   "scan <directory>",
		Short: "Scan component library and generate catalog",
		Long:  "Scans a component library directory, extracts props, detects patterns, and generates a catalog JSON file.",
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{"group": "core"},
		RunE: func(cmd *cobra.Command, args []string) error {
			directory := args[0]

			buildCfg := scanner.CatalogBuildConfig{
				Name:         name,
				ImportPrefix: importPrefix,
				RootDir:      directory,
			}

			if categoriesStr != "" {
				rules, err := scanner.ParseCategoryRules(categoriesStr)
				if err != nil {
					return fmt.Errorf("invalid --categories value: %w", err)
				}
				buildCfg.CategoryRules = rules
			}

			scanCfg := scanner.DefaultScanConfig()
			scanCfg.NoEnrich = noEnrich

			s := scanner.NewScanner(nil)
			defer s.Close()

			cat, stats, err := scanWithSpinner(s, directory, scanCfg, buildCfg)
			if err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			// Determine output path.
			if output == "" {
				catalogName := cat.Name
				if catalogName == "" {
					catalogName = "catalog"
				}
				output = fmt.Sprintf(".uispec/catalogs/%s.json", catalogName)
			}

			// Write catalog JSON.
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			data, err := json.MarshalIndent(cat, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal catalog: %w", err)
			}

			if err := os.WriteFile(output, data, 0644); err != nil {
				return fmt.Errorf("failed to write catalog: %w", err)
			}

			// --init: update .uispec/config.yaml with catalog path.
			if initConfig {
				if err := updateOrCreateConfig(output); err != nil {
					fmt.Fprintf(os.Stderr, "warning: --init failed: %v\n", err)
				} else if outCfg.Verbosity != VerbQuiet {
					fmt.Println("Updated .uispec/config.yaml")
				}
			}

			// Quiet mode: just print the output path.
			if outCfg.Verbosity == VerbQuiet {
				fmt.Println(output)
				return nil
			}

			// Phase timeline.
			fmt.Println()
			fmt.Println(renderPhaseLine(
				fmt.Sprintf("Discovered %d files", stats.FilesDiscovered),
				stats.DiscoveryTimeMs))
			fmt.Println(renderPhaseLine(
				fmt.Sprintf("Extracted symbols from %d files", stats.FilesExtracted),
				stats.ExtractionTimeMs))
			fmt.Println(renderPhaseLine(
				fmt.Sprintf("Detected %d components", stats.ComponentsDetected),
				stats.DetectionTimeMs))
			fmt.Println(renderPhaseLine(
				fmt.Sprintf("Extracted %d props", stats.PropsExtracted),
				stats.PropExtractionTimeMs))
			if stats.EnrichmentTimeMs > 0 {
				fmt.Println(renderPhaseLine(
					fmt.Sprintf("Enriched %d components", stats.EnrichedComponents),
					stats.EnrichmentTimeMs))
			}
			if stats.TokenExtractionTimeMs > 0 {
				fmt.Println(renderPhaseLine(
					fmt.Sprintf("Extracted %d tokens", stats.TokensExtracted),
					stats.TokenExtractionTimeMs))
			}
			if stats.StorybookExtractionTimeMs > 0 {
				fmt.Println(renderPhaseLine(
					fmt.Sprintf("Extracted %d examples", stats.ExamplesExtracted),
					stats.StorybookExtractionTimeMs))
			}
			fmt.Println(renderPhaseLine(
				fmt.Sprintf("Built catalog (%d components)", len(cat.Components)),
				stats.CatalogBuildTimeMs))

			// Summary line.
			parts := []string{
				styleSuccess(fmt.Sprintf("%d", len(cat.Components))) + " components",
				styleSuccess(fmt.Sprintf("%d", stats.PropsExtracted)) + " props",
			}
			if stats.TokensExtracted > 0 {
				parts = append(parts, styleSuccess(fmt.Sprintf("%d", stats.TokensExtracted))+" tokens")
			}
			if stats.ExamplesExtracted > 0 {
				parts = append(parts, styleSuccess(fmt.Sprintf("%d", stats.ExamplesExtracted))+" examples")
			}
			fmt.Printf("\n  %s %s\n", styleSuccess("✓"), strings.Join(parts, " · "))

			// Output path with file size.
			sizeStr := ""
			if fi, err := os.Stat(output); err == nil {
				sizeKB := fi.Size() / 1024
				sizeStr = styleDim(fmt.Sprintf("(%d KB)", sizeKB))
			}
			fmt.Printf("    Wrote %s %s\n", styleBold(output), sizeStr)

			// Warnings.
			gapCount := 0
			for _, comp := range cat.Components {
				if len(comp.Props) == 0 {
					gapCount++
				}
			}
			if gapCount > 0 {
				fmt.Printf("    %s %d component(s) have no props\n", styleWarning("!"), gapCount)
			}
			if stats.FilesFailed > 0 {
				fmt.Printf("    %s %d file(s) failed to extract\n", styleWarning("!"), stats.FilesFailed)
			}

			// Next steps.
			steps := []NextStep{
				{"uispec serve", "Start MCP server for AI agents"},
			}
			if len(cat.Components) > 0 {
				steps = append(steps, NextStep{
					fmt.Sprintf("uispec inspect %s", cat.Components[0].Name),
					"Inspect a component",
				})
			}
			renderNextSteps(steps)
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().StringVar(&output, "output", "", "Output file path (default: .uispec/catalogs/<name>.json)")
	cmd.Flags().StringVar(&name, "name", "", "Catalog name")
	cmd.Flags().StringVar(&importPrefix, "import-prefix", "", "Import prefix for components")
	cmd.Flags().StringVar(&categoriesStr, "categories", "", "Category rules (format: name=glob,...)")
	cmd.Flags().BoolVar(&noEnrich, "no-enrich", false, "Skip TypeScript enrichment")
	cmd.Flags().BoolVar(&initConfig, "init", false, "Update .uispec/config.yaml with catalog path")

	return cmd
}
