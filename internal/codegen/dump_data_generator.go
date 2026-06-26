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
	"go/format"
	"strings"
)

// TableDump holds the data for a single table to be upserted in a dump-data migration.
type TableDump struct {
	Table        string           // target table name
	ConflictKeys []string         // PK or unique columns for ON CONFLICT
	Rows         []map[string]any // row data (all rows same keys)
}

// DumpDataGenerator generates Go migration source containing UpsertData operations.
type DumpDataGenerator struct{}

// NewDumpDataGenerator creates a new DumpDataGenerator.
func NewDumpDataGenerator() *DumpDataGenerator {
	return &DumpDataGenerator{}
}

// Generate produces a complete .go migration file source containing UpsertData
// operations for each table dump. name is the migration name (e.g. "0003_dump_countries"),
// deps is the list of dependency migration names, and tables must contain at least one entry.
func (g *DumpDataGenerator) Generate(name string, deps []string, tables []TableDump) (string, error) {
	if len(tables) == 0 {
		return "", fmt.Errorf("at least one table dump is required")
	}

	var b strings.Builder

	b.WriteString("package main\n\n")
	b.WriteString("import m \"github.com/ocomsoft/morphic/migrate\"\n\n")
	b.WriteString("func init() {\n")
	fmt.Fprintf(&b, "\tm.Register(&m.Migration{\n")
	fmt.Fprintf(&b, "\t\tName:         %q,\n", name)

	// Dependencies
	depStrs := make([]string, len(deps))
	for i, d := range deps {
		depStrs[i] = fmt.Sprintf("%q", d)
	}
	fmt.Fprintf(&b, "\t\tDependencies: []string{%s},\n", strings.Join(depStrs, ", "))

	// Operations
	b.WriteString("\t\tOperations: []m.Operation{\n")
	for _, td := range tables {
		if err := g.writeUpsertData(&b, td); err != nil {
			return "", fmt.Errorf("writing UpsertData for table %q: %w", td.Table, err)
		}
	}
	b.WriteString("\t\t},\n")

	b.WriteString("\t})\n")
	b.WriteString("}\n")

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("formatting dump-data migration: %w\nRaw:\n%s", err, b.String())
	}
	return string(formatted), nil
}

// writeUpsertData writes a single &m.UpsertData{...} literal to the builder.
func (g *DumpDataGenerator) writeUpsertData(b *strings.Builder, td TableDump) error {
	fmt.Fprintf(b, "\t\t\t&m.UpsertData{\n")
	fmt.Fprintf(b, "\t\t\t\tTable: %q,\n", td.Table)

	// ConflictKeys
	keyStrs := make([]string, len(td.ConflictKeys))
	for i, k := range td.ConflictKeys {
		keyStrs[i] = fmt.Sprintf("%q", k)
	}
	fmt.Fprintf(b, "\t\t\t\tConflictKeys: []string{%s},\n", strings.Join(keyStrs, ", "))

	// Rows
	b.WriteString("\t\t\t\tRows: []map[string]any{\n")
	for _, row := range td.Rows {
		b.WriteString("\t\t\t\t\t{\n")
		for _, key := range sortedMapKeys(row) {
			fmt.Fprintf(b, "\t\t\t\t\t\t%q: %s,\n", key, formatGoLiteral(row[key]))
		}
		b.WriteString("\t\t\t\t\t},\n")
	}
	b.WriteString("\t\t\t\t},\n")

	b.WriteString("\t\t\t},\n")
	return nil
}

// GenerateStarlark produces a .star migration file source containing upsert_data
// operations for each table dump.
func (g *DumpDataGenerator) GenerateStarlark(name string, deps []string, tables []TableDump) (string, error) {
	if len(tables) == 0 {
		return "", fmt.Errorf("at least one table dump is required")
	}

	var b strings.Builder

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

// formatGoLiteral converts a Go value to its Go source literal representation.
func formatGoLiteral(v any) string {
	if v == nil {
		return "nil"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case int:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}
