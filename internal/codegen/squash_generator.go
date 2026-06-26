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

package codegen

import (
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/migrate"
)

// SquashGenerator generates squashed migration files.
// A squashed migration combines multiple migrations into one, listing the originals
// in its Replaces field so the runner can skip them if already applied.
type SquashGenerator struct{}

// NewSquashGenerator creates a new SquashGenerator.
func NewSquashGenerator() *SquashGenerator {
	return &SquashGenerator{}
}

// GenerateStarlarkSquash generates a .star squashed migration file.
func (g *SquashGenerator) GenerateStarlarkSquash(
	name string,
	replaces []string,
	migrations []*migrate.Migration,
) (string, error) {
	var b strings.Builder

	b.WriteString(GenerationHeader("#", "generate squash"))
	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	b.WriteString("    dependencies = [],\n")

	replaceStrs := make([]string, len(replaces))
	for i, r := range replaces {
		replaceStrs[i] = fmt.Sprintf("%q", r)
	}
	fmt.Fprintf(&b, "    replaces = [%s],\n", strings.Join(replaceStrs, ", "))

	b.WriteString("    operations = [\n")
	for _, mig := range migrations {
		for _, op := range mig.Operations {
			opStr, err := convertOperation(op)
			if err != nil {
				return "", fmt.Errorf("converting operation from %q to Starlark: %w", mig.Name, err)
			}
			b.WriteString(opStr)
		}
	}
	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}
