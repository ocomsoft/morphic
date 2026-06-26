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
	"fmt"
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

// GenerateStarlarkMerge generates a .star merge migration file.
func (g *MergeGenerator) GenerateStarlarkMerge(name string, deps []string) (string, error) {
	var b strings.Builder

	b.WriteString(GenerationHeader("#", "generate merge"))
	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [],\n")
	b.WriteString(")\n")
	return b.String(), nil
}
