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

// Package codegen generates Go source files for the migration framework.
package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
)

// MergeGenerator generates merge migration .go files.
// Merge migrations have no operations — they exist only to establish a
// common ancestor for two divergent branches of the migration DAG.
type MergeGenerator struct{}

// NewMergeGenerator creates a new MergeGenerator.
func NewMergeGenerator() *MergeGenerator {
	return &MergeGenerator{}
}

// GenerateMerge generates the source code for a merge migration .go file.
// name is the migration name (e.g. "0004_merge_feature_a_and_b").
// deps is the list of branch leaf names to merge.
func (g *MergeGenerator) GenerateMerge(name string, deps []string) (string, error) {
	var buf bytes.Buffer

	buf.WriteString("package main\n\n")
	buf.WriteString("import m \"github.com/ocomsoft/morphic/migrate\"\n\n")
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\tm.Register(&m.Migration{\n")
	fmt.Fprintf(&buf, "\t\tName:         %q,\n", name)

	depStrs := make([]string, len(deps))
	for i, d := range deps {
		depStrs[i] = fmt.Sprintf("%q", d)
	}
	fmt.Fprintf(&buf, "\t\tDependencies: []string{%s},\n", strings.Join(depStrs, ", "))
	buf.WriteString("\t\tOperations:   []m.Operation{},\n")
	buf.WriteString("\t})\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("formatting merge migration: %w\nRaw:\n%s", err, buf.String())
	}
	return string(formatted), nil
}

// GenerateStarlarkMerge generates a .star merge migration file.
func (g *MergeGenerator) GenerateStarlarkMerge(name string, deps []string) (string, error) {
	var b strings.Builder

	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [],\n")
	b.WriteString(")\n")
	return b.String(), nil
}
