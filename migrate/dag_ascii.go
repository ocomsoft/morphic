/*
MIT License

# Copyright (c) 2025 OcomSoft

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package migrate

import (
	"fmt"
	"strings"
)

// RenderDAGASCII produces a human-readable ASCII tree of the migration graph.
// It uses box-drawing characters to show parent->child relationships.
func RenderDAGASCII(out *DAGOutput) string {
	if out == nil || len(out.Migrations) == 0 {
		return "No migrations registered.\n"
	}

	var sb strings.Builder
	sb.WriteString("Migration Graph\n")
	sb.WriteString("===============\n\n")

	// Build a quick lookup: name -> summary
	byName := make(map[string]MigrationSummary, len(out.Migrations))
	for _, m := range out.Migrations {
		byName[m.Name] = m
	}

	// Build reverse lookup: name -> children (names that depend on this one)
	children := make(map[string][]string)
	for _, m := range out.Migrations {
		for _, dep := range m.Dependencies {
			children[dep] = append(children[dep], m.Name)
		}
	}

	// Render roots first, then recurse into children
	rendered := make(map[string]bool)
	var render func(name, prefix string, isLast bool)
	render = func(name, prefix string, isLast bool) {
		if rendered[name] {
			return
		}
		rendered[name] = true
		m := byName[name]

		connector := "|->"
		childPrefix := prefix + "|  "
		if isLast {
			connector = "\\->"
			childPrefix = prefix + "   "
		}

		if prefix == "" {
			fmt.Fprintf(&sb, "  %s\n", name)
		} else {
			fmt.Fprintf(&sb, "%s %s %s\n", prefix, connector, name)
		}

		// Print operations
		opPrefix := childPrefix
		if prefix == "" {
			opPrefix = "  |  "
		}
		for _, op := range m.Operations {
			fmt.Fprintf(&sb, "%s%s\n", opPrefix, op.Description)
		}

		// Recurse into children
		ch := children[name]
		for i, child := range ch {
			render(child, childPrefix, i == len(ch)-1)
		}
		if len(ch) > 0 {
			fmt.Fprintf(&sb, "%s|\n", opPrefix)
		}
	}

	for _, root := range out.Roots {
		render(root, "", true)
	}

	fmt.Fprintf(&sb, "\nRoots:  %s\n", strings.Join(out.Roots, ", "))
	fmt.Fprintf(&sb, "Leaves: %s\n", strings.Join(out.Leaves, ", "))
	if out.HasBranches {
		sb.WriteString("WARNING: Branches detected -- run morphic generate --merge\n")
	} else {
		sb.WriteString("OK: No branches -- graph is linear\n")
	}
	return sb.String()
}
