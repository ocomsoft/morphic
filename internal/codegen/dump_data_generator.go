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
)

// TableDump holds the data for a single table to be upserted in a dump-data migration.
type TableDump struct {
	Table        string           // target table name
	ConflictKeys []string         // PK or unique columns for ON CONFLICT
	Rows         []map[string]any // row data (all rows same keys)
}

// DumpDataGenerator generates Starlark migration source containing UpsertData operations.
type DumpDataGenerator struct{}

// NewDumpDataGenerator creates a new DumpDataGenerator.
func NewDumpDataGenerator() *DumpDataGenerator {
	return &DumpDataGenerator{}
}

// GenerateStarlark produces a .star migration file source containing upsert_data
// operations for each table dump.
func (g *DumpDataGenerator) GenerateStarlark(name string, deps []string, tables []TableDump) (string, error) {
	if len(tables) == 0 {
		return "", fmt.Errorf("at least one table dump is required")
	}

	var b strings.Builder

	b.WriteString(GenerationHeader("#", "generate dump-data"))
	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [\n")

	for _, td := range tables {
		g.writeStarlarkUpsertData(&b, td)
	}

	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// writeStarlarkUpsertData writes a single upsert_data(...) call to the builder.
func (g *DumpDataGenerator) writeStarlarkUpsertData(b *strings.Builder, td TableDump) {
	fmt.Fprintf(b, "        upsert_data(%q,\n", td.Table)

	conflictStrs := make([]string, len(td.ConflictKeys))
	for i, k := range td.ConflictKeys {
		conflictStrs[i] = fmt.Sprintf("%q", k)
	}
	fmt.Fprintf(b, "            conflict_keys = [%s],\n", strings.Join(conflictStrs, ", "))

	b.WriteString("            rows = [\n")
	for _, row := range td.Rows {
		b.WriteString("                row(")
		keys := sortedMapKeys(row)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, formatStarlarkLiteral(row[k])))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString("),\n")
	}
	b.WriteString("            ],\n")
	b.WriteString("        ),\n")
}

// GenerateStarlarkWithRowsFile produces a .star migration file where each
// upsert_data operation references an external JSONL file via rows_file
// instead of inlining the row data.
func (g *DumpDataGenerator) GenerateStarlarkWithRowsFile(name string, deps []string, tables []TableDump) (string, error) {
	if len(tables) == 0 {
		return "", fmt.Errorf("at least one table dump is required")
	}

	var b strings.Builder

	b.WriteString(GenerationHeader("#", "generate dump-data --json"))
	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [\n")

	for _, td := range tables {
		g.writeStarlarkUpsertDataWithRowsFile(&b, name, td)
	}

	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// writeStarlarkUpsertDataWithRowsFile writes a upsert_data() call with
// rows_file instead of inline rows.
func (g *DumpDataGenerator) writeStarlarkUpsertDataWithRowsFile(b *strings.Builder, migrationName string, td TableDump) {
	fmt.Fprintf(b, "        upsert_data(%q,\n", td.Table)

	conflictStrs := make([]string, len(td.ConflictKeys))
	for i, k := range td.ConflictKeys {
		conflictStrs[i] = fmt.Sprintf("%q", k)
	}
	fmt.Fprintf(b, "            conflict_keys = [%s],\n", strings.Join(conflictStrs, ", "))

	jsonlFile := fmt.Sprintf("%s_%s.jsonl", migrationName, td.Table)
	fmt.Fprintf(b, "            rows_file = %q,\n", jsonlFile)
	b.WriteString("        ),\n")
}
