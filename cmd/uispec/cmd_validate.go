package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gnana997/uispec/pkg/parser"
	"github.com/gnana997/uispec/pkg/validator"
)

func newValidateCmd() *cobra.Command {
	var catalogFlag string
	var autoFix, asJSON bool

	cmd := &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate code against catalog rules",
		Long:  "Validates a TypeScript/JSX file against the component catalog, checking for incorrect prop usage and other violations.",
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{"group": "core"},
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]

			if asJSON {
				outCfg.IsJSON = true
			}

			code, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("cannot read file: %w", err)
			}

			catalogPath := resolveCatalogPath(catalogFlag)
			qs, err := loadCatalog(catalogPath)
			if err != nil {
				return err
			}

			pm := parser.NewParserManager(nil)
			defer func() { _ = pm.Close() }()
			v := validator.NewValidator(qs.Catalog, qs.Index, pm)

			result := v.ValidatePage(string(code), autoFix)

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(result); err != nil {
					return fmt.Errorf("failed to encode result: %w", err)
				}
				if !result.Valid {
					return &ExitError{Code: 2, Err: fmt.Errorf("validation failed")}
				}
				return nil
			}

			// Quiet mode.
			if outCfg.Verbosity == VerbQuiet {
				if result.Valid {
					fmt.Println(filePath)
				} else {
					fmt.Printf("%s: %d violation(s)\n", filePath, len(result.Violations))
				}
				if !result.Valid {
					return &ExitError{Code: 2, Err: fmt.Errorf("validation failed")}
				}
				return nil
			}

			// Human-readable output.
			if result.Valid {
				fmt.Printf("%s %s — no violations\n",
					styleSuccess("✓"), styleDim(filePath))
			} else {
				fmt.Printf("%s %s — %d violation(s)\n",
					styleError("✗"), styleBold(filePath), len(result.Violations))
				for _, viol := range result.Violations {
					sev := strings.ToUpper(viol.Severity[:1]) + viol.Severity[1:]
					sevStyled := styleError(fmt.Sprintf("[%s]", sev))
					if strings.ToLower(viol.Severity) == "warning" {
						sevStyled = styleWarning(fmt.Sprintf("[%s]", sev))
					}
					fmt.Printf("  %s line %d:%d  %s  %s\n",
						sevStyled, viol.Line, viol.Column, viol.Message, styleDim("("+viol.Rule+")"))
					if viol.Suggestion != "" {
						fmt.Printf("         %s %s\n", styleDim("→"), viol.Suggestion)
					}
				}
			}

			if autoFix && len(result.Fixes) > 0 {
				fmt.Printf("\n%s fix(es) applied — writing %s\n",
					styleSuccess(fmt.Sprintf("%d", len(result.Fixes))), filePath)
				if err := os.WriteFile(filePath, []byte(result.FixedCode), 0644); err != nil {
					return fmt.Errorf("failed to write fixed file: %w", err)
				}
			}

			if !result.Valid {
				return &ExitError{Code: 2, Err: fmt.Errorf("validation failed")}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&catalogFlag, "catalog", "", "Path to component catalog")
	cmd.Flags().BoolVar(&autoFix, "fix", false, "Auto-fix violations")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON instead of human-readable")

	return cmd
}
