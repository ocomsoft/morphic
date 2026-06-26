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
package struct2schema

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/types"
)

// Writer handles writing schema files and previewing output
type Writer struct {
	verbose bool
}

// NewWriter creates a new schema writer
func NewWriter(verbose bool) *Writer {
	return &Writer{
		verbose: verbose,
	}
}

// WriteSchema writes a schema to a Starlark DSL file
func (w *Writer) WriteSchema(schema *types.Schema, outputPath string) error {
	if w.verbose {
		fmt.Printf("Writing schema to: %s\n", outputPath)
	}

	// Create backup of existing file if it exists
	if err := w.backupExistingFile(outputPath); err != nil {
		return fmt.Errorf("failed to backup existing file: %w", err)
	}

	// Check if existing schema should be merged
	existingSchema, err := w.loadExistingSchema(outputPath)
	if err != nil && w.verbose {
		fmt.Printf("Note: Could not load existing schema: %v\n", err)
	}

	// Merge with existing schema if present
	if existingSchema != nil {
		mergedSchema, err := w.mergeSchemas(existingSchema, schema)
		if err != nil {
			return fmt.Errorf("failed to merge with existing schema: %w", err)
		}
		schema = mergedSchema
		if w.verbose {
			fmt.Println("Merged with existing schema")
		}
	}

	// Validate the schema
	if err := schema.Validate(); err != nil {
		return fmt.Errorf("generated schema is invalid: %w", err)
	}

	// Generate Starlark DSL
	dslData, err := codegen.GenerateSchemaDSL(schema)
	if err != nil {
		return fmt.Errorf("failed to generate Starlark DSL: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(dslData), 0644); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	if w.verbose {
		fmt.Printf("Successfully wrote schema with %d table(s)\n", len(schema.Tables))
	}

	return nil
}

// PreviewSchema prints the schema to stdout for dry-run mode
func (w *Writer) PreviewSchema(schema *types.Schema) error {
	dslData, err := codegen.GenerateSchemaDSL(schema)
	if err != nil {
		return fmt.Errorf("failed to generate Starlark DSL: %w", err)
	}

	fmt.Print(dslData)
	return nil
}

// loadExistingSchema loads an existing schema file if it exists.
// Supports both .star (Starlark DSL) and .yaml formats.
func (w *Writer) loadExistingSchema(path string) (*types.Schema, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil // File doesn't exist, not an error
	}

	dir := filepath.Dir(path)
	return interp.LoadSchema(dir, w.verbose)
}

// backupExistingFile creates a backup of an existing file
func (w *Writer) backupExistingFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // No file to backup
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := path + ".backup." + timestamp

	// Copy the file
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return err
	}

	if w.verbose {
		fmt.Printf("Created backup: %s\n", backupPath)
	}

	return nil
}

// mergeSchemas merges a new schema with an existing one
func (w *Writer) mergeSchemas(existing, newSchema *types.Schema) (*types.Schema, error) {
	// Start with the new schema as base
	merged := &types.Schema{
		Database: newSchema.Database,
		Defaults: newSchema.Defaults,
		Include:  append(existing.Include, newSchema.Include...),
		Tables:   []types.Table{},
	}

	// Keep database info from existing if new doesn't have it
	if merged.Database.Name == "generated_schema" && existing.Database.Name != "" {
		merged.Database.Name = existing.Database.Name
	}
	if merged.Database.Version == "1.0.0" && existing.Database.Version != "" {
		merged.Database.Version = existing.Database.Version
	}

	// Create a map of existing tables for quick lookup
	existingTables := make(map[string]*types.Table)
	for i := range existing.Tables {
		table := &existing.Tables[i]
		existingTables[table.Name] = table
	}

	// Process new tables
	newTableNames := make(map[string]bool)
	for _, newTable := range newSchema.Tables {
		newTableNames[newTable.Name] = true

		if existingTable, exists := existingTables[newTable.Name]; exists {
			// Merge table definitions
			mergedTable, err := w.mergeTables(existingTable, &newTable)
			if err != nil {
				return nil, fmt.Errorf("failed to merge table %s: %w", newTable.Name, err)
			}
			merged.Tables = append(merged.Tables, *mergedTable)
			if w.verbose {
				fmt.Printf("Merged table: %s\n", newTable.Name)
			}
		} else {
			// Add new table
			merged.Tables = append(merged.Tables, newTable)
			if w.verbose {
				fmt.Printf("Added new table: %s\n", newTable.Name)
			}
		}
	}

	// Add existing tables that weren't in the new schema
	for _, existingTable := range existing.Tables {
		if !newTableNames[existingTable.Name] {
			merged.Tables = append(merged.Tables, existingTable)
			if w.verbose {
				fmt.Printf("Preserved existing table: %s\n", existingTable.Name)
			}
		}
	}

	return merged, nil
}

// mergeTables merges two table definitions
func (w *Writer) mergeTables(existing, newTable *types.Table) (*types.Table, error) {
	merged := &types.Table{
		Name:    newTable.Name,
		Fields:  []types.Field{},
		Indexes: []types.Index{},
	}

	// Create map of existing fields
	existingFields := make(map[string]*types.Field)
	for i := range existing.Fields {
		field := &existing.Fields[i]
		existingFields[field.Name] = field
	}

	// Process new fields
	newFieldNames := make(map[string]bool)
	for _, newField := range newTable.Fields {
		newFieldNames[newField.Name] = true

		if existingField, exists := existingFields[newField.Name]; exists {
			// Keep existing field but update with new info if needed
			mergedField := w.mergeFields(existingField, &newField)
			merged.Fields = append(merged.Fields, *mergedField)
		} else {
			// Add new field
			merged.Fields = append(merged.Fields, newField)
		}
	}

	// Add existing fields that weren't in new schema
	for _, existingField := range existing.Fields {
		if !newFieldNames[existingField.Name] {
			merged.Fields = append(merged.Fields, existingField)
		}
	}

	// Merge indexes (prefer new ones, but keep existing if not conflicting)
	indexNames := make(map[string]bool)
	for _, newIndex := range newTable.Indexes {
		merged.Indexes = append(merged.Indexes, newIndex)
		indexNames[newIndex.Name] = true
	}

	for _, existingIndex := range existing.Indexes {
		if !indexNames[existingIndex.Name] {
			merged.Indexes = append(merged.Indexes, existingIndex)
		}
	}

	return merged, nil
}

// mergeFields merges two field definitions, preferring the new one but preserving manual changes
func (w *Writer) mergeFields(existing, newField *types.Field) *types.Field {
	// Start with new field as base
	merged := *newField

	// Preserve some existing attributes that might have been manually set
	if existing.Default != "" && newField.Default == "" {
		merged.Default = existing.Default
	}

	return &merged
}

// CreateOutputDirectory creates the output directory if it doesn't exist
func (w *Writer) CreateOutputDirectory(outputPath string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	return nil
}

// ValidateOutputPath validates that the output path is suitable for writing
func (w *Writer) ValidateOutputPath(outputPath string) error {
	// Check if directory is writable
	dir := filepath.Dir(outputPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Directory doesn't exist, try to create it
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create output directory: %w", err)
		}
	}

	// Check if we can write to the directory
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("cannot write to output directory: %w", err)
	}
	_ = os.Remove(testFile) // Clean up

	return nil
}
