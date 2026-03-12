package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
			sevStyled := styleDim(fmt.Sprintf("[%s]", g.Severity))
			if g.Severity == "error" {
				sevStyled = styleError(fmt.Sprintf("[%s]", g.Severity))
			} else if g.Severity == "warning" {
				sevStyled = styleWarning(fmt.Sprintf("[%s]", g.Severity))
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

// printPropsSectionStyled renders the props table with dynamic column widths and color.
func printPropsSectionStyled(title string, props []catalog.Prop) {
	if len(props) == 0 {
		fmt.Printf("%s  %s\n", styleHeader(title), styleDim("(none)"))
		return
	}

	fmt.Println(styleHeader(title))

	// Compute column widths.
	nameW := len("NAME")
	typeW := len("TYPE")
	defW := len("DEFAULT")
	for _, p := range props {
		if len(p.Name) > nameW {
			nameW = len(p.Name)
		}
		if len(p.Type) > typeW {
			typeW = len(p.Type)
		}
		def := p.Default
		if def == "" {
			def = "—"
		}
		if len(def) > defW {
			defW = len(def)
		}
	}

	// Header row.
	sepLen := nameW + typeW + 5 + defW + 4
	fmt.Printf("  %s\n", styleDim(fmt.Sprintf("%-*s  %-*s  %-3s  %-*s", nameW, "NAME", typeW, "TYPE", "REQ", defW, "DEFAULT")))
	fmt.Printf("  %s\n", styleDim(strings.Repeat("─", sepLen)))

	// Prop rows.
	for _, p := range props {
		req := styleDim("no")
		if p.Required {
			req = styleSuccess("yes")
		}
		def := p.Default
		if def == "" {
			def = "—"
		}
		deprecated := ""
		if p.Deprecated {
			deprecated = " " + styleWarning("[deprecated]")
		}
		// Use raw strings for alignment, then style.
		fmt.Printf("  %-*s  %-*s  %-3s  %-*s%s\n",
			nameW, p.Name, typeW, p.Type, req, defW, def, deprecated)

		if p.Description != "" {
			fmt.Printf("  %s  %s\n", strings.Repeat(" ", nameW), styleDim(p.Description))
		}
		if len(p.AllowedValues) > 0 {
			allowed := strings.Join(p.AllowedValues, " | ")
			label := strings.Repeat(" ", nameW)
			fmt.Printf("  %s  %s\n", label, styleDim("allowed: "+wrapAllowed(allowed, nameW+12)))
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
