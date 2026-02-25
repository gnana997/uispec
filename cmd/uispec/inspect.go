package main

import (
	"fmt"
	"strings"

	"github.com/gnana997/uispec/pkg/catalog"
)

const maxWidth = 80

// printComponentHuman prints a human-readable component summary to stdout.
func printComponentHuman(comp *catalog.Component, isSubComp bool, requestedName string, showExamples bool) {
	// Header: name + category + deprecated notice
	header := comp.Name
	if isSubComp {
		header = fmt.Sprintf("%s  (sub-component of %s)", requestedName, comp.Name)
	}
	if comp.Deprecated {
		header += "  [DEPRECATED]"
	}
	fmt.Printf("%s  [%s]\n", header, comp.Category)

	if comp.Deprecated && comp.DeprecatedMsg != "" {
		fmt.Printf("  Deprecated: %s\n", comp.DeprecatedMsg)
	}

	// Description
	if comp.Description != "" {
		fmt.Println()
		printWrapped(comp.Description, 0, maxWidth)
	}

	// Import
	fmt.Println()
	fmt.Println("Import")
	if len(comp.ImportedNames) > 0 {
		names := strings.Join(comp.ImportedNames, ", ")
		fmt.Printf("  from %q import { %s }\n", comp.ImportPath, names)
	} else {
		fmt.Printf("  from %q\n", comp.ImportPath)
	}

	// Props (top-level)
	fmt.Println()
	printPropsSection("Props", comp.Props)

	// Sub-component props (if we showed a specific sub-component)
	if isSubComp {
		for _, sub := range comp.SubComponents {
			if strings.EqualFold(sub.Name, requestedName) && len(sub.Props) > 0 {
				fmt.Println()
				printPropsSection(requestedName+" Props", sub.Props)
				break
			}
		}
	}

	// Sub-components
	fmt.Println()
	if len(comp.SubComponents) == 0 {
		fmt.Println("Sub-components  (none)")
	} else {
		fmt.Println("Sub-components")
		nameWidth := 0
		for _, sub := range comp.SubComponents {
			if len(sub.Name) > nameWidth {
				nameWidth = len(sub.Name)
			}
		}
		for _, sub := range comp.SubComponents {
			padding := strings.Repeat(" ", nameWidth-len(sub.Name))
			fmt.Printf("  %s%s  %s\n", sub.Name, padding, sub.Description)
		}
	}

	// Guidelines
	fmt.Println()
	if len(comp.Guidelines) == 0 {
		fmt.Println("Guidelines  (none)")
	} else {
		fmt.Println("Guidelines")
		for _, g := range comp.Guidelines {
			sev := fmt.Sprintf("[%s]", g.Severity)
			fmt.Printf("  %-9s %s\n", sev, g.Description)
		}
	}

	// Examples (opt-in)
	if showExamples {
		fmt.Println()
		if len(comp.Examples) == 0 {
			fmt.Println("Examples  (none)")
		} else {
			fmt.Println("Examples")
			for _, ex := range comp.Examples {
				fmt.Println()
				fmt.Printf("  %s\n", ex.Title)
				if ex.Description != "" {
					fmt.Printf("  %s\n", ex.Description)
				}
				fmt.Println("  " + strings.Repeat("─", 40))
				for _, line := range strings.Split(ex.Code, "\n") {
					fmt.Printf("  %s\n", line)
				}
			}
		}
	}
}

// printPropsSection renders the props table with dynamic column widths.
func printPropsSection(title string, props []catalog.Prop) {
	if len(props) == 0 {
		fmt.Printf("%s  (none)\n", title)
		return
	}

	fmt.Println(title)

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
	sepLen := nameW + typeW + 5 + defW + 4 // NAME + TYPE + "REQ" + DEFAULT + spacing
	fmt.Printf("  %-*s  %-*s  %-3s  %-*s\n", nameW, "NAME", typeW, "TYPE", "REQ", defW, "DEFAULT")
	fmt.Printf("  %s\n", strings.Repeat("─", sepLen))

	// Prop rows.
	for _, p := range props {
		req := "no"
		if p.Required {
			req = "yes"
		}
		def := p.Default
		if def == "" {
			def = "—"
		}
		deprecated := ""
		if p.Deprecated {
			deprecated = " [deprecated]"
		}
		fmt.Printf("  %-*s  %-*s  %-3s  %-*s%s\n",
			nameW, p.Name, typeW, p.Type, req, defW, def, deprecated)

		if p.Description != "" {
			fmt.Printf("  %s  %s\n", strings.Repeat(" ", nameW), p.Description)
		}
		if len(p.AllowedValues) > 0 {
			allowed := strings.Join(p.AllowedValues, " | ")
			label := strings.Repeat(" ", nameW)
			fmt.Printf("  %s  allowed: %s\n", label, wrapAllowed(allowed, nameW+12))
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
