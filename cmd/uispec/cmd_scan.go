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

			// Print summary.
			fmt.Printf("\nScanned %s files in %s\n\n",
				styleSuccess(fmt.Sprintf("%d", stats.FilesDiscovered)), directory)
			fmt.Printf("%s %s\n", styleHeader("Components:"), styleSuccess(fmt.Sprintf("%d", len(cat.Components))))
			for _, comp := range cat.Components {
				propsCount := len(comp.Props)
				subInfo := ""
				if len(comp.SubComponents) > 0 {
					subNames := make([]string, len(comp.SubComponents))
					for i, sc := range comp.SubComponents {
						subNames[i] = sc.Name
					}
					subInfo = fmt.Sprintf(" + %d sub-components (%s)", len(comp.SubComponents), strings.Join(subNames, ", "))
				}
				fmt.Printf("  %s %s%s\n", comp.Name, styleDim(fmt.Sprintf("(%d props)", propsCount)), subInfo)
			}

			// Props breakdown.
			if stats.PropsFromEnrichment > 0 {
				fmt.Printf("\nProps extracted: %s (tree-sitter: %d, enriched: +%d)\n",
					styleSuccess(fmt.Sprintf("%d", stats.PropsExtracted)),
					stats.PropsFromTreeSitter, stats.PropsFromEnrichment)
			} else {
				fmt.Printf("\nProps extracted: %s\n", styleSuccess(fmt.Sprintf("%d", stats.PropsExtracted)))
			}

			if stats.TokensExtracted > 0 {
				fmt.Printf("Tokens extracted: %s\n", styleSuccess(fmt.Sprintf("%d", stats.TokensExtracted)))
			}

			if stats.ExamplesExtracted > 0 {
				fmt.Printf("Examples extracted: %s\n", styleSuccess(fmt.Sprintf("%d", stats.ExamplesExtracted)))
			}

			// Gaps: components with no props.
			gapCount := 0
			for _, comp := range cat.Components {
				if len(comp.Props) == 0 {
					gapCount++
				}
			}
			if gapCount > 0 {
				fmt.Printf("%s %d component(s) have no props\n", styleWarning("Gaps:"), gapCount)
			}

			if stats.FilesFailed > 0 {
				fmt.Printf("%s %d file(s) failed to extract\n", styleWarning("Warning:"), stats.FilesFailed)
			}

			fmt.Printf("\nWrote %s\n", styleBold(output))

			// Timing: verbose only.
			if outCfg.Verbosity == VerbVerbose {
				timing := fmt.Sprintf("Timing: discovery %dms, extraction %dms, detection %dms, props %dms",
					stats.DiscoveryTimeMs, stats.ExtractionTimeMs,
					stats.DetectionTimeMs, stats.PropExtractionTimeMs)
				if stats.EnrichmentTimeMs > 0 {
					timing += fmt.Sprintf(", enrichment %dms", stats.EnrichmentTimeMs)
				}
				if stats.TokenExtractionTimeMs > 0 {
					timing += fmt.Sprintf(", tokens %dms", stats.TokenExtractionTimeMs)
				}
				if stats.StorybookExtractionTimeMs > 0 {
					timing += fmt.Sprintf(", storybook %dms", stats.StorybookExtractionTimeMs)
				}
				timing += fmt.Sprintf(", build %dms (total %dms)",
					stats.CatalogBuildTimeMs, stats.TotalTimeMs)
				fmt.Println(styleDim(timing))
			}

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
