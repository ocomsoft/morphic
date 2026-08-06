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
package yaml

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/ocomsoft/morphic/internal/errors"
)

// Merger handles YAML schema merging with conflict resolution
type Merger struct {
	verbose bool
}

// NewMerger creates a new YAML merger
func NewMerger(verbose bool) *Merger {
	return &Merger{
		verbose: verbose,
	}
}

// MergeSchemas merges multiple YAML schemas into a single unified schema
func (m *Merger) MergeSchemas(schemas []*Schema) (*Schema, error) {
	if len(schemas) == 0 {
		return nil, errors.NewValidationError("merger", "no schemas provided for merging")
	}

	if len(schemas) == 1 {
		return schemas[0], nil
	}

	// Start with the first schema as base
	merged := &Schema{
		Database:     schemas[0].Database,
		Defaults:     m.mergeDefaults(schemas),
		TypeMappings: m.mergeTypeMappings(schemas),
		Tables:       make([]Table, 0),
	}

	// Collect all tables from all schemas.
	tableMap := make(map[string][]Table)
	for _, schema := range schemas {
		for _, table := range schema.Tables {
			tableMap[table.Name] = append(tableMap[table.Name], table)
		}
	}

	// Merge tables
	for tableName, tables := range tableMap {
		mergedTable, err := m.mergeTables(tableName, tables)
		if err != nil {
			return nil, fmt.Errorf("failed to merge table %s: %w", tableName, err)
		}
		merged.Tables = append(merged.Tables, *mergedTable)
	}

	if m.verbose {
		fmt.Printf("Merged %d schemas into single schema with %d tables\n", len(schemas), len(merged.Tables))
	}

	return merged, nil
}

// mergeDefaults merges default value mappings from multiple schemas.
// Later schemas override earlier ones for the same key.
func (m *Merger) mergeDefaults(schemas []*Schema) Defaults {
	merged := make(Defaults)

	for _, schema := range schemas {
		for dbType, providerDefaults := range schema.Defaults {
			if merged[dbType] == nil {
				merged[dbType] = make(map[string]string)
			}
			for key, value := range providerDefaults {
				merged[dbType][key] = value
			}
		}
	}

	return merged
}

// mergeTypeMappings merges type mapping overrides from multiple schemas.
// Later schemas override earlier ones for the same key.
func (m *Merger) mergeTypeMappings(schemas []*Schema) TypeMappings {
	merged := make(TypeMappings)

	for _, schema := range schemas {
		for dbType, providerMappings := range schema.TypeMappings {
			if merged[dbType] == nil {
				merged[dbType] = make(map[string]string)
			}
			for key, value := range providerMappings {
				merged[dbType][key] = value
			}
		}
	}

	return merged
}

// mergeTables merges multiple table definitions with the same name
func (m *Merger) mergeTables(tableName string, tables []Table) (*Table, error) {
	if len(tables) == 1 {
		return &tables[0], nil
	}

	_, _ = fmt.Fprint(os.Stderr, color.YellowString("Warning: merging %d definitions for table: %s\n", len(tables), tableName))
	for _, t := range tables {
		if t.SourceFile != "" {
			_, _ = fmt.Fprint(os.Stderr, color.YellowString("  - %s\n", t.SourceFile))
		}
	}

	merged := &Table{
		Name:   tableName,
		Fields: make([]Field, 0),
	}

	// Collect all fields from all table definitions
	fieldMap := make(map[string][]Field)
	for _, table := range tables {
		for _, field := range table.Fields {
			fieldMap[field.Name] = append(fieldMap[field.Name], field)
		}
	}

	// Merge fields
	for fieldName, fields := range fieldMap {
		mergedField, err := m.mergeFields(tableName, fieldName, fields)
		if err != nil {
			return nil, fmt.Errorf("failed to merge field %s: %w", fieldName, err)
		}
		merged.Fields = append(merged.Fields, *mergedField)
	}

	// Merge indexes — deduplicate by name, last definition wins.
	indexMap := make(map[string]Index)
	for _, table := range tables {
		for _, idx := range table.Indexes {
			indexMap[idx.Name] = idx
		}
	}
	for _, idx := range indexMap {
		merged.Indexes = append(merged.Indexes, idx)
	}

	return merged, nil
}

// mergeFields merges multiple field definitions with the same name using conflict resolution rules
func (m *Merger) mergeFields(tableName, fieldName string, fields []Field) (*Field, error) {
	if len(fields) == 1 {
		return &fields[0], nil
	}

	if m.verbose {
		fmt.Printf("  Merging %d definitions for field: %s.%s\n", len(fields), tableName, fieldName)
	}

	// Start with the first field as base
	merged := fields[0]

	// Apply conflict resolution rules
	for i := 1; i < len(fields); i++ {
		current := fields[i]

		// Type conflict resolution
		if merged.Type != current.Type {
			resolvedType, err := m.resolveTypeConflict(tableName, fieldName, merged.Type, current.Type)
			if err != nil {
				return nil, err
			}
			merged.Type = resolvedType
		}

		// VARCHAR/text length conflict resolution (larger wins)
		if (merged.Type == "varchar" || merged.Type == "text") && current.Length > merged.Length {
			merged.Length = current.Length
			if m.verbose {
				fmt.Printf("    Resolved length conflict: using larger length %d\n", current.Length)
			}
		}

		// Decimal precision/scale conflict resolution (larger precision wins)
		if merged.Type == "decimal" {
			if current.Precision > merged.Precision {
				merged.Precision = current.Precision
				merged.Scale = current.Scale
				if m.verbose {
					fmt.Printf("    Resolved decimal conflict: using larger precision %d,%d\n", current.Precision, current.Scale)
				}
			}
		}

		// Nullable conflict resolution (NOT NULL wins)
		if !merged.IsNullable() || !current.IsNullable() {
			merged.SetNullable(false)
			if m.verbose {
				fmt.Printf("    Resolved nullable conflict: NOT NULL wins\n")
			}
		}

		// Primary key conflict resolution (any true wins)
		if current.PrimaryKey {
			merged.PrimaryKey = true
		}

		// Auto fields conflict resolution (any true wins)
		if current.AutoCreate {
			merged.AutoCreate = true
		}
		if current.AutoUpdate {
			merged.AutoUpdate = true
		}

		// Default value conflict resolution (non-empty wins, later definition wins if both non-empty)
		if current.Default != "" {
			if merged.Default == "" || merged.Default != current.Default {
				merged.Default = current.Default
				if m.verbose {
					fmt.Printf("    Resolved default conflict: using '%s'\n", current.Default)
				}
			}
		}

		// Foreign key conflict resolution
		if current.ForeignKey != nil {
			if merged.ForeignKey == nil {
				merged.ForeignKey = current.ForeignKey
			} else {
				// Validate that foreign key definitions are compatible
				if merged.ForeignKey.Table != current.ForeignKey.Table {
					return nil, fmt.Errorf("incompatible foreign key definitions for %s.%s: references %s vs %s",
						tableName, fieldName, merged.ForeignKey.Table, current.ForeignKey.Table)
				}
			}
		}

		// Many-to-many conflict resolution
		if current.ManyToMany != nil {
			if merged.ManyToMany == nil {
				merged.ManyToMany = current.ManyToMany
			} else {
				// Validate that many-to-many definitions are compatible
				if merged.ManyToMany.Table != current.ManyToMany.Table {
					return nil, fmt.Errorf("incompatible many-to-many definitions for %s.%s: references %s vs %s",
						tableName, fieldName, merged.ManyToMany.Table, current.ManyToMany.Table)
				}
			}
		}
	}

	return &merged, nil
}

// resolveTypeConflict resolves conflicts between different field types
func (m *Merger) resolveTypeConflict(tableName, fieldName, type1, type2 string) (string, error) {
	// Allow compatible type promotions
	compatibleTypes := map[string][]string{
		"integer": {"bigint"},  // integer can be promoted to bigint
		"varchar": {"text"},    // varchar can be promoted to text
		"float":   {"decimal"}, // float can be promoted to decimal (with precision loss warning)
	}

	// Check if one type can be promoted to another
	if canPromote(type1, type2, compatibleTypes) {
		if m.verbose {
			fmt.Printf("    Resolved type conflict: promoting %s to %s\n", type1, type2)
		}
		return type2, nil
	}

	if canPromote(type2, type1, compatibleTypes) {
		if m.verbose {
			fmt.Printf("    Resolved type conflict: promoting %s to %s\n", type2, type1)
		}
		return type1, nil
	}

	// If no compatible promotion is possible, this is an error
	return "", fmt.Errorf("incompatible field types for %s.%s: %s vs %s", tableName, fieldName, type1, type2)
}

// canPromote checks if fromType can be promoted to toType
func canPromote(fromType, toType string, compatibleTypes map[string][]string) bool {
	if promotions, exists := compatibleTypes[fromType]; exists {
		for _, promotion := range promotions {
			if promotion == toType {
				return true
			}
		}
	}
	return false
}

// ValidateMergedSchema validates the merged schema for consistency
func (m *Merger) ValidateMergedSchema(schema *Schema) error {
	// Basic schema validation
	if err := schema.Validate(); err != nil {
		return err
	}

	// Validate that each table has at most one primary key
	for _, table := range schema.Tables {
		primaryKeyCount := 0
		for _, field := range table.Fields {
			if field.PrimaryKey {
				primaryKeyCount++
			}
		}
		if primaryKeyCount > 1 {
			return fmt.Errorf("table %s has multiple primary keys", table.Name)
		}
	}

	// Validate foreign key references exist
	parser := NewParser(m.verbose)
	if err := parser.ValidateForeignKeyReferences(schema); err != nil {
		return err
	}

	return nil
}

// GetMergedTableNames returns the names of all tables that were merged from multiple definitions
func (m *Merger) GetMergedTableNames(schemas []*Schema) []string {
	tableCount := make(map[string]int)

	for _, schema := range schemas {
		for _, table := range schema.Tables {
			tableCount[table.Name]++
		}
	}

	var mergedTables []string
	for tableName, count := range tableCount {
		if count > 1 {
			mergedTables = append(mergedTables, tableName)
		}
	}

	return mergedTables
}

// GetMergedFieldNames returns the names of all fields that were merged within each table
func (m *Merger) GetMergedFieldNames(_ string, tables []Table) []string {
	fieldCount := make(map[string]int)

	for _, table := range tables {
		for _, field := range table.Fields {
			fieldCount[field.Name]++
		}
	}

	var mergedFields []string
	for fieldName, count := range fieldCount {
		if count > 1 {
			mergedFields = append(mergedFields, fieldName)
		}
	}

	return mergedFields
}
