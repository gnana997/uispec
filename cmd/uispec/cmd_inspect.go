package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"

	"github.com/gnana997/uispec/pkg/catalog"
)

func newInspectCmd() *cobra.Command {
	var catalogFlag string
	var asJSON, showExamples bool

	cmd := &cobra.Command{
		Use:   "inspect <component>",
		Short: "Inspect a component's props and usage",
		Long:  "Shows detailed information about a component including props, sub-components, guidelines, and examples.",
		Args:  cobra.ExactArgs(1),
		Annotations: map[string]string{"group": "info"},
		RunE: func(cmd *cobra.Command, args []string) error {
			componentName := args[0]

			if asJSON {
				outCfg.IsJSON = true
			}

			catalogPath := resolveCatalogPath(catalogFlag)
			qs, err := loadCatalog(catalogPath)
			if err != nil {
				return err
			}

			comp, found := qs.GetComponent(componentName)
			if !found {
				return fmt.Errorf("component %q not found in catalog", componentName)
			}

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(comp); err != nil {
					return fmt.Errorf("failed to encode component: %w", err)
				}
				return nil
			}

			// Quiet mode: just name + prop count.
			if outCfg.Verbosity == VerbQuiet {
				fmt.Printf("%s (%d props)\n", comp.Name, len(comp.Props))
				return nil
			}

			// Is this a sub-component lookup?
			_, isTopLevel := qs.Index.ComponentByName[componentName]
			isSubComp := !isTopLevel

			printComponentHumanStyled(comp, isSubComp, componentName, showExamples)
			return nil
		},
	}

	cmd.Flags().StringVar(&catalogFlag, "catalog", "", "Path to component catalog")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON instead of human-readable")
	cmd.Flags().BoolVar(&showExamples, "examples", false, "Show code examples")

	return cmd
}

const maxWidth = 80

// printComponentHumanStyled prints a human-readable component summary with color.
func printComponentHumanStyled(comp *catalog.Component, isSubComp bool, requestedName string, showExamples bool) {
	// Header: name + category + deprecated notice
	header := styleHeader(comp.Name)
	if isSubComp {
		header = fmt.Sprintf("%s  %s", styleHeader(requestedName), styleDim(fmt.Sprintf("(sub-component of %s)", comp.Name)))
	}
	if comp.Deprecated {
		header += "  " + styleWarning("[DEPRECATED]")
	}
	fmt.Printf("%s  %s\n", header, styleDim("["+comp.Category+"]"))

	if comp.Deprecated && comp.DeprecatedMsg != "" {
		fmt.Printf("  %s %s\n", styleWarning("Deprecated:"), comp.DeprecatedMsg)
	}

	// Description
	if comp.Description != "" {
		fmt.Println()
		printWrapped(comp.Description, 0, maxWidth)
	}

	// Import
	fmt.Println()
	fmt.Println(styleHeader("Import"))
	if len(comp.ImportedNames) > 0 {
		names := strings.Join(comp.ImportedNames, ", ")
		fmt.Printf("  from %q import { %s }\n", comp.ImportPath, names)
	} else {
		fmt.Printf("  from %q\n", comp.ImportPath)
	}

	// Props (top-level)
	fmt.Println()
	printPropsSectionStyled("Props", comp.Props)

	// Sub-component props (if we showed a specific sub-component)
	if isSubComp {
		for _, sub := range comp.SubComponents {
			if strings.EqualFold(sub.Name, requestedName) && len(sub.Props) > 0 {
				fmt.Println()
				printPropsSectionStyled(requestedName+" Props", sub.Props)
				break
			}
		}
	}

	// Sub-components
	fmt.Println()
	if len(comp.SubComponents) == 0 {
		fmt.Printf("%s  %s\n", styleHeader("Sub-components"), styleDim("(none)"))
	} else {
		fmt.Println(styleHeader("Sub-components"))
		nameWidth := 0
		for _, sub := range comp.SubComponents {
			if len(sub.Name) > nameWidth {
				nameWidth = len(sub.Name)
			}
		}
		for _, sub := range comp.SubComponents {
			padding := strings.Repeat(" ", nameWidth-len(sub.Name))
			fmt.Printf("  %s%s  %s\n", sub.Name, padding, styleDim(sub.Description))
		}
	}

	// Guidelines
	fmt.Println()
	if len(comp.Guidelines) == 0 {
		fmt.Printf("%s  %s\n", styleHeader("Guidelines"), styleDim("(none)"))
	} else {
		fmt.Println(styleHeader("Guidelines"))
		for _, g := range comp.Guidelines {
			tag := fmt.Sprintf("[%s]", g.Severity)
			var sevStyled string
			switch g.Severity {
			case "error":
				sevStyled = styleError(tag)
			case "warning":
				sevStyled = styleWarning(tag)
			default:
				sevStyled = styleDim(tag)
			}
			fmt.Printf("  %-9s %s\n", sevStyled, g.Description)
		}
	}

	// Examples (opt-in)
	if showExamples {
		fmt.Println()
		if len(comp.Examples) == 0 {
			fmt.Printf("%s  %s\n", styleHeader("Examples"), styleDim("(none)"))
		} else {
			fmt.Println(styleHeader("Examples"))
			for _, ex := range comp.Examples {
				fmt.Println()
				fmt.Printf("  %s\n", styleBold(ex.Title))
				if ex.Description != "" {
					fmt.Printf("  %s\n", styleDim(ex.Description))
				}
				fmt.Println("  " + styleDim(strings.Repeat("─", 40)))
				for _, line := range strings.Split(ex.Code, "\n") {
					fmt.Printf("  %s\n", line)
				}
			}
		}
	}
}

// printPropsSectionStyled renders the props table using lipgloss/table.
func printPropsSectionStyled(title string, props []catalog.Prop) {
	if len(props) == 0 {
		fmt.Printf("%s  %s\n", styleHeader(title), styleDim("(none)"))
		return
	}

	fmt.Println(styleHeader(title))

	// Build table rows.
	t := ltable.New().
		Headers("PROP", "TYPE", "REQ", "DEFAULT").
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("8"))).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			if outCfg.NoColor {
				return s
			}
			if row == ltable.HeaderRow {
				return s.Bold(true).Foreground(lipgloss.Color("8"))
			}
			// REQ column styling.
			if col == 2 && row >= 0 && row < len(props) {
				if props[row].Required {
					return s.Foreground(lipgloss.Color("2")) // green
				}
				return s.Foreground(lipgloss.Color("8")) // dim
			}
			return s
		})

	for _, p := range props {
		req := "no"
		if p.Required {
			req = "yes"
		}
		def := p.Default
		if def == "" {
			def = "—"
		}
		name := p.Name
		if p.Deprecated {
			name += " " + styleWarning("[deprecated]")
		}
		t.Row(name, p.Type, req, def)
	}

	// Indent the table by 2 spaces.
	for _, line := range strings.Split(t.Render(), "\n") {
		fmt.Printf("  %s\n", line)
	}

	// Print descriptions and allowed values below the table.
	for _, p := range props {
		if p.Description != "" || len(p.AllowedValues) > 0 {
			fmt.Printf("  %s: ", styleBold(p.Name))
			if p.Description != "" {
				fmt.Print(styleDim(p.Description))
			}
			if len(p.AllowedValues) > 0 {
				if p.Description != "" {
					fmt.Print(" ")
				}
				allowed := strings.Join(p.AllowedValues, " | ")
				fmt.Print(styleDim("allowed: " + allowed))
			}
			fmt.Println()
		}
	}
}

// wrapAllowed wraps the allowed values string if it exceeds maxWidth.
func wrapAllowed(allowed string, indent int) string {
	if indent+len(allowed) <= maxWidth {
		return allowed
	}
	parts := strings.Split(allowed, " | ")
	var sb strings.Builder
	lineLen := indent
	for i, part := range parts {
		addition := len(part)
		if i > 0 {
			addition += 3 // " | "
		}
		if lineLen+addition > maxWidth && i > 0 {
			sb.WriteString("\n")
			sb.WriteString(strings.Repeat(" ", indent))
			lineLen = indent
		}
		if i > 0 {
			sb.WriteString(" | ")
			lineLen += 3
		}
		sb.WriteString(part)
		lineLen += len(part)
	}
	return sb.String()
}

// printWrapped prints text word-wrapped at maxWidth with the given left indent.
func printWrapped(text string, indent, width int) {
	words := strings.Fields(text)
	prefix := strings.Repeat(" ", indent)
	line := prefix
	for _, word := range words {
		if len(line)+len(word)+1 > width && line != prefix {
			fmt.Println(line)
			line = prefix + word
		} else {
			if line == prefix {
				line += word
			} else {
				line += " " + word
			}
		}
	}
	if line != prefix {
		fmt.Println(line)
	}
}
