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

// schema_dsl_generator.go converts a *types.Schema into a Starlark schema
// DSL (.star) file. It reuses the field/index/fk formatting helpers from
// starlark_generator.go.
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ocomsoft/morphic/internal/types"
)

// GenerateSchemaDSL converts a *types.Schema into the Starlark schema DSL
// source text. The output is a complete .star file that can be evaluated by
// SchemaDSLBuiltins to reconstruct the same schema.
func GenerateSchemaDSL(schema *types.Schema) (string, error) {
	if schema == nil {
		return "", fmt.Errorf("schema is nil")
	}

	var b strings.Builder

	// database()
	if schema.Database.Version != "" {
		fmt.Fprintf(&b, "database(%q, %q)\n", schema.Database.Name, schema.Database.Version)
	} else {
		fmt.Fprintf(&b, "database(%q)\n", schema.Database.Name)
	}

	// includes
	for _, inc := range schema.Include {
		fmt.Fprintf(&b, "\ninclude(%q, %q)\n", inc.Module, inc.Path)
	}

	// defaults
	emitSortedMappings(&b, "defaults", schema.Defaults)

	// type_mappings
	emitSortedTypeMappings(&b, "type_mappings", schema.TypeMappings)

	// tables
	for _, table := range schema.Tables {
		b.WriteString("\n")
		emitTable(&b, table)
	}

	return b.String(), nil
}

// emitSortedMappings writes defaults() calls sorted by database type.
func emitSortedMappings(b *strings.Builder, fnName string, d types.Defaults) {
	if len(d) == 0 {
		return
	}
	keys := sortedDBTypes(d)
	for _, dbType := range keys {
		mapping := d[dbType]
		b.WriteString("\n")
		fmt.Fprintf(b, "%s(%q, %s)\n", fnName, string(dbType), formatStarlarkStringDict(mapping))
	}
}

// emitSortedTypeMappings writes type_mappings() calls sorted by database type.
func emitSortedTypeMappings(b *strings.Builder, fnName string, tm types.TypeMappings) {
	if len(tm) == 0 {
		return
	}
	keys := sortedTMTypes(tm)
	for _, dbType := range keys {
		mapping := tm[dbType]
		b.WriteString("\n")
		fmt.Fprintf(b, "%s(%q, %s)\n", fnName, string(dbType), formatStarlarkStringDict(mapping))
	}
}

// emitTable writes a single table() call.
func emitTable(b *strings.Builder, table types.Table) {
	fmt.Fprintf(b, "table(%q,\n", table.Name)

	if len(table.Fields) > 0 {
		b.WriteString("    fields = [\n")
		for _, f := range table.Fields {
			fmt.Fprintf(b, "        %s,\n", generateStarlarkField(f))
		}
		b.WriteString("    ],\n")
	}

	// Indexes (exclude FromFK).
	var userIndexes []types.Index
	for _, idx := range table.Indexes {
		if !idx.FromFK {
			userIndexes = append(userIndexes, idx)
		}
	}
	if len(userIndexes) > 0 {
		b.WriteString("    indexes = [\n")
		for _, idx := range userIndexes {
			fmt.Fprintf(b, "        %s,\n", generateStarlarkIndex(idx))
		}
		b.WriteString("    ],\n")
	}

	b.WriteString(")\n")
}

// sortedDBTypes returns Defaults keys sorted alphabetically.
func sortedDBTypes(d types.Defaults) []types.DatabaseType {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	result := make([]types.DatabaseType, len(keys))
	for i, k := range keys {
		result[i] = types.DatabaseType(k)
	}
	return result
}

// sortedTMTypes returns TypeMappings keys sorted alphabetically.
func sortedTMTypes(tm types.TypeMappings) []types.DatabaseType {
	keys := make([]string, 0, len(tm))
	for k := range tm {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	result := make([]types.DatabaseType, len(keys))
	for i, k := range keys {
		result[i] = types.DatabaseType(k)
	}
	return result
}
