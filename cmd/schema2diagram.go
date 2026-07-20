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
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/workflow"
)

var (
	// Output flags for schema2diagram
	diagramOutput string
)

// schema2diagramCmd represents the schema2diagram command
var schema2diagramCmd = &cobra.Command{
	Use:     "schema-to-diagram",
	Aliases: []string{"schema2diagram"},
	GroupID: "inspect",
	Short:   "Generate Markdown documentation with diagrams from schema files",
	Long: `Generate comprehensive Markdown documentation with Entity Relationship Diagrams (ERD)
from schema files.

This command scans all schema files in module dependencies, merges them into a
unified schema, and generates detailed Markdown documentation including:

- Complete table documentation with field details
- Mermaid Entity Relationship Diagrams (ERD)
- Index documentation and relationships
- Foreign key constraints and relationships
- Data type specifications and constraints

Documentation Features:
- Interactive Mermaid diagrams compatible with GitHub, GitLab, and documentation tools
- Complete field specifications with types, constraints, and defaults
- Index and constraint documentation
- Relationship mapping between tables
- Professional formatting suitable for technical documentation

The generated documentation is ideal for:
- Project documentation and onboarding
- Database design reviews and discussions  
- Technical specifications and architecture documents
- Code review and collaboration
- API documentation and developer guides

Output Format:
The command generates a single Markdown file with embedded Mermaid diagrams
that can be viewed in any Markdown viewer that supports Mermaid (GitHub, GitLab,
VS Code, etc.).

Examples:
  # Generate documentation to default file
  morphic schema2diagram

  # Generate to specific output file
  morphic schema2diagram --output=docs/database-schema.md

  # Generate with verbose processing information
  morphic schema2diagram --verbose --output=schema-docs.md

The generated Markdown includes live diagrams that automatically update when
viewed in supported platforms, making it perfect for living documentation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchema2Diagram(cmd, args)
	},
}

// runSchema2Diagram executes the schema2diagram command
func runSchema2Diagram(cmd *cobra.Command, args []string) error {
	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Generating schema documentation with diagrams\n")
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "===========================================\n")
	}

	// Use PostgreSQL as default for diagram generation (type doesn't affect diagram output)
	dbType := types.DatabasePostgreSQL

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Output file: %s\n", diagramOutput)
	}

	// Initialize YAML components using shared workflow functions
	components := workflow.InitializeYAMLComponents(dbType, verbose)

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n1. Scanning for schema files...\n")
	}

	// Scan and parse schemas using shared function
	allSchemas, err := workflow.ScanAndParseSchemas(components, verbose)
	if err != nil {
		if err.Error() == "no YAML schema files found" {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "No schema files found. Nothing to document.\n")
			return nil
		}
		return err
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n2. Parsing and merging schemas...\n")
	}

	// Merge and validate schemas using shared function
	mergedSchema, err := workflow.MergeAndValidateSchemas(components, allSchemas, dbType, verbose)
	if err != nil {
		return fmt.Errorf("merged schema validation failed: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Merged schema: %d tables\n", len(mergedSchema.Tables))
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n3. Generating Markdown documentation...\n")
	}

	// Generate Markdown documentation with diagrams
	markdown, err := generateMarkdownDocumentation(mergedSchema)
	if err != nil {
		return fmt.Errorf("failed to generate documentation: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n4. Writing documentation file...\n")
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(diagramOutput)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write Markdown file
	if err := os.WriteFile(diagramOutput, []byte(markdown), 0644); err != nil {
		return fmt.Errorf("failed to write documentation file: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Schema documentation successfully generated: %s\n", diagramOutput)

	if len(mergedSchema.Tables) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nDocumented %d tables with Entity Relationship Diagram\n", len(mergedSchema.Tables))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe generated Markdown includes:\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  • Interactive Mermaid ERD diagram\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  • Complete table and field documentation\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  • Index and constraint specifications\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  • Relationship mapping between tables\n")
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nView the documentation in any Markdown viewer with Mermaid support.\n")

	return nil
}

// generateMarkdownDocumentation creates comprehensive Markdown documentation with diagrams
func generateMarkdownDocumentation(schema *types.Schema) (string, error) {
	var md strings.Builder

	// Header
	md.WriteString("# Database Schema Documentation\n\n")
	fmt.Fprintf(&md, "**Database:** %s  \n", schema.Database.Name)
	fmt.Fprintf(&md, "**Version:** %s  \n", schema.Database.Version)
	fmt.Fprintf(&md, "**Generated:** %s  \n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Table of Contents
	md.WriteString("## Table of Contents\n\n")
	md.WriteString("- [Entity Relationship Diagram](#entity-relationship-diagram)\n")
	md.WriteString("- [Schema Overview](#schema-overview)\n")
	md.WriteString("- [Table Documentation](#table-documentation)\n")

	// Sort tables for consistent output
	sortedTables := make([]types.Table, len(schema.Tables))
	copy(sortedTables, schema.Tables)
	sort.Slice(sortedTables, func(i, j int) bool {
		return sortedTables[i].Name < sortedTables[j].Name
	})

	for _, table := range sortedTables {
		fmt.Fprintf(&md, "  - [%s Table](#%s-table)\n",
			cases.Title(language.Und).String(table.Name), strings.ToLower(strings.ReplaceAll(table.Name, "_", "-")))
	}
	md.WriteString("- [Indexes and Constraints](#indexes-and-constraints)\n")
	md.WriteString("- [Relationships](#relationships)\n\n")

	// Entity Relationship Diagram
	if err := generateERDSection(&md, schema); err != nil {
		return "", fmt.Errorf("failed to generate ERD section: %w", err)
	}

	// Schema Overview
	generateOverviewSection(&md, schema)

	// Table Documentation
	generateTableDocumentation(&md, sortedTables)

	// Indexes and Constraints
	generateIndexesSection(&md, sortedTables)

	// Relationships
	generateRelationshipsSection(&md, sortedTables)

	return md.String(), nil
}

// generateERDSection creates the Entity Relationship Diagram using Mermaid
func generateERDSection(md *strings.Builder, schema *types.Schema) error {
	md.WriteString("## Entity Relationship Diagram\n\n")
	md.WriteString("```mermaid\n")
	md.WriteString("erDiagram\n")

	// Sort tables for consistent output
	sortedTables := make([]types.Table, len(schema.Tables))
	copy(sortedTables, schema.Tables)
	sort.Slice(sortedTables, func(i, j int) bool {
		return sortedTables[i].Name < sortedTables[j].Name
	})

	// Define entities with their fields
	for _, table := range sortedTables {
		fmt.Fprintf(md, "    %s {\n", strings.ToUpper(table.Name))

		// Sort fields for consistent output
		sortedFields := make([]types.Field, len(table.Fields))
		copy(sortedFields, table.Fields)
		sort.Slice(sortedFields, func(i, j int) bool {
			return sortedFields[i].Name < sortedFields[j].Name
		})

		for _, field := range sortedFields {
			fieldType := convertTypeForMermaid(field.Type)
			constraints := generateFieldConstraints(field)

			fmt.Fprintf(md, "        %s %s %s\n",
				fieldType, field.Name, constraints)
		}
		md.WriteString("    }\n\n")
	}

	// Define relationships
	for _, table := range sortedTables {
		for _, field := range table.Fields {
			if field.Type == "foreign_key" && field.ForeignKey != nil {
				relationship := generateRelationshipType(field.ForeignKey.OnDelete)
				fmt.Fprintf(md, "    %s %s %s : \"%s\"\n",
					strings.ToUpper(field.ForeignKey.Table),
					relationship,
					strings.ToUpper(table.Name),
					field.Name)
			}
		}
	}

	md.WriteString("```\n\n")
	md.WriteString("*The diagram above shows the complete entity relationship structure. ")
	md.WriteString("Each entity represents a database table with its fields and constraints. ")
	md.WriteString("Relationships between tables are shown with connecting lines.*\n\n")

	return nil
}

// generateOverviewSection creates the schema overview section
func generateOverviewSection(md *strings.Builder, schema *types.Schema) {
	md.WriteString("## Schema Overview\n\n")

	// Count statistics
	totalFields := 0
	totalIndexes := 0
	totalForeignKeys := 0

	for _, table := range schema.Tables {
		totalFields += len(table.Fields)
		totalIndexes += len(table.Indexes)
		for _, field := range table.Fields {
			if field.Type == "foreign_key" {
				totalForeignKeys++
			}
		}
	}

	md.WriteString("| Statistic | Count |\n")
	md.WriteString("|-----------|-------|\n")
	fmt.Fprintf(md, "| **Total Tables** | %d |\n", len(schema.Tables))
	fmt.Fprintf(md, "| **Total Fields** | %d |\n", totalFields)
	fmt.Fprintf(md, "| **Total Indexes** | %d |\n", totalIndexes)
	fmt.Fprintf(md, "| **Foreign Key Relationships** | %d |\n\n", totalForeignKeys)

	// Database defaults
	pgDefaults := schema.Defaults.ForProvider(types.DatabasePostgreSQL)
	if len(pgDefaults) > 0 {
		md.WriteString("### Default Values\n\n")
		md.WriteString("The schema defines the following default value mappings:\n\n")
		md.WriteString("| Default Key | PostgreSQL Value |\n")
		md.WriteString("|-------------|------------------|\n")

		// Sort defaults for consistent output
		var defaultKeys []string
		for key := range pgDefaults {
			defaultKeys = append(defaultKeys, key)
		}
		sort.Strings(defaultKeys)

		for _, key := range defaultKeys {
			value := pgDefaults[key]
			fmt.Fprintf(md, "| `%s` | `%s` |\n", key, value)
		}
		md.WriteString("\n")
	}
}

// generateTableDocumentation creates detailed documentation for each table
func generateTableDocumentation(md *strings.Builder, tables []types.Table) {
	md.WriteString("## Table Documentation\n\n")

	for _, table := range tables {
		tableName := cases.Title(language.Und).String(table.Name)

		fmt.Fprintf(md, "### %s Table\n\n", tableName)
		fmt.Fprintf(md, "**Table Name:** `%s`  \n", table.Name)
		fmt.Fprintf(md, "**Field Count:** %d  \n", len(table.Fields))
		fmt.Fprintf(md, "**Index Count:** %d  \n\n", len(table.Indexes))

		// Fields documentation
		md.WriteString("#### Fields\n\n")
		md.WriteString("| Field Name | Type | Constraints | Default | Description |\n")
		md.WriteString("|------------|------|-------------|---------|-------------|\n")

		// Sort fields for consistent output
		sortedFields := make([]types.Field, len(table.Fields))
		copy(sortedFields, table.Fields)
		sort.Slice(sortedFields, func(i, j int) bool {
			return sortedFields[i].Name < sortedFields[j].Name
		})

		for _, field := range sortedFields {
			fieldType := generateFieldTypeDescription(field)
			constraints := generateFieldConstraintsDescription(field)
			defaultValue := generateDefaultDescription(field)
			description := generateFieldDescription(field)

			fmt.Fprintf(md, "| `%s` | %s | %s | %s | %s |\n",
				field.Name, fieldType, constraints, defaultValue, description)
		}
		md.WriteString("\n")

		// Indexes for this table
		if len(table.Indexes) > 0 {
			md.WriteString("#### Indexes\n\n")
			md.WriteString("| Index Name | Fields | Type | Purpose |\n")
			md.WriteString("|------------|--------|------|----------|\n")

			// Sort indexes for consistent output
			sortedIndexes := make([]types.Index, len(table.Indexes))
			copy(sortedIndexes, table.Indexes)
			sort.Slice(sortedIndexes, func(i, j int) bool {
				return sortedIndexes[i].Name < sortedIndexes[j].Name
			})

			for _, index := range sortedIndexes {
				indexType := "Index"
				purpose := "Query optimization"
				if index.Unique {
					indexType = "Unique Index"
					purpose = "Uniqueness constraint + query optimization"
				}

				fieldsStr := strings.Join(index.Fields, ", ")
				fmt.Fprintf(md, "| `%s` | `%s` | %s | %s |\n",
					index.Name, fieldsStr, indexType, purpose)
			}
			md.WriteString("\n")
		}

		// Foreign key relationships
		foreignKeys := []types.Field{}
		for _, field := range table.Fields {
			if field.Type == "foreign_key" {
				foreignKeys = append(foreignKeys, field)
			}
		}

		if len(foreignKeys) > 0 {
			md.WriteString("#### Foreign Key Relationships\n\n")
			md.WriteString("| Field | References | On Delete | Purpose |\n")
			md.WriteString("|-------|------------|-----------|----------|\n")

			for _, fk := range foreignKeys {
				onDelete := "PROTECT"
				if fk.ForeignKey != nil {
					onDelete = fk.ForeignKey.OnDelete
				}
				purpose := fmt.Sprintf("Links to %s table", fk.ForeignKey.Table)

				fmt.Fprintf(md, "| `%s` | `%s` | %s | %s |\n",
					fk.Name, fk.ForeignKey.Table, onDelete, purpose)
			}
			md.WriteString("\n")
		}

		md.WriteString("---\n\n")
	}
}

// generateIndexesSection creates comprehensive index documentation
func generateIndexesSection(md *strings.Builder, tables []types.Table) {
	md.WriteString("## Indexes and Constraints\n\n")

	// Collect all indexes
	var allIndexes []struct {
		TableName string
		Index     types.Index
	}

	for _, table := range tables {
		for _, index := range table.Indexes {
			allIndexes = append(allIndexes, struct {
				TableName string
				Index     types.Index
			}{
				TableName: table.Name,
				Index:     index,
			})
		}
	}

	if len(allIndexes) == 0 {
		md.WriteString("*No explicit indexes defined in the schema.*\n\n")
		return
	}

	// Sort indexes by table name, then by index name
	sort.Slice(allIndexes, func(i, j int) bool {
		if allIndexes[i].TableName != allIndexes[j].TableName {
			return allIndexes[i].TableName < allIndexes[j].TableName
		}
		return allIndexes[i].Index.Name < allIndexes[j].Index.Name
	})

	md.WriteString("### All Indexes\n\n")
	md.WriteString("| Table | Index Name | Fields | Type | Performance Impact |\n")
	md.WriteString("|-------|------------|--------|----|-------------------|\n")

	for _, item := range allIndexes {
		indexType := "B-tree Index"
		impact := "Improves SELECT performance on indexed fields"
		if item.Index.Unique {
			indexType = "Unique B-tree Index"
			impact = "Enforces uniqueness + improves SELECT performance"
		}

		fieldsStr := "`" + strings.Join(item.Index.Fields, "`, `") + "`"
		fmt.Fprintf(md, "| `%s` | `%s` | %s | %s | %s |\n",
			item.TableName, item.Index.Name, fieldsStr, indexType, impact)
	}
	md.WriteString("\n")

	// Index recommendations
	md.WriteString("### Index Recommendations\n\n")
	md.WriteString("- **Unique indexes** enforce data integrity and provide fast lookups\n")
	md.WriteString("- **Composite indexes** (multiple fields) are most effective when query conditions match the field order\n")
	md.WriteString("- **Foreign key fields** should have indexes for optimal JOIN performance\n")
	md.WriteString("- **Frequently queried fields** benefit from dedicated indexes\n\n")
}

// generateRelationshipsSection creates relationship documentation
func generateRelationshipsSection(md *strings.Builder, tables []types.Table) {
	md.WriteString("## Relationships\n\n")

	// Collect all relationships
	var relationships []struct {
		FromTable string
		FromField string
		ToTable   string
		OnDelete  string
	}

	for _, table := range tables {
		for _, field := range table.Fields {
			if field.Type == "foreign_key" && field.ForeignKey != nil {
				relationships = append(relationships, struct {
					FromTable string
					FromField string
					ToTable   string
					OnDelete  string
				}{
					FromTable: table.Name,
					FromField: field.Name,
					ToTable:   field.ForeignKey.Table,
					OnDelete:  field.ForeignKey.OnDelete,
				})
			}
		}
	}

	if len(relationships) == 0 {
		md.WriteString("*No foreign key relationships defined in the schema.*\n\n")
		return
	}

	// Sort relationships by from table, then to table
	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].FromTable != relationships[j].FromTable {
			return relationships[i].FromTable < relationships[j].FromTable
		}
		return relationships[i].ToTable < relationships[j].ToTable
	})

	md.WriteString("### Foreign Key Relationships\n\n")
	md.WriteString("| From Table | Field | To Table | On Delete | Relationship Type |\n")
	md.WriteString("|------------|-------|----------|-----------|-------------------|\n")

	for _, rel := range relationships {
		relationshipType := determineRelationshipType(rel.OnDelete)

		fmt.Fprintf(md, "| `%s` | `%s` | `%s` | %s | %s |\n",
			rel.FromTable, rel.FromField, rel.ToTable, rel.OnDelete, relationshipType)
	}
	md.WriteString("\n")

	// Relationship explanations
	md.WriteString("### Relationship Explanations\n\n")
	md.WriteString("- **CASCADE**: When parent record is deleted, child records are automatically deleted\n")
	md.WriteString("- **RESTRICT**: Prevents deletion of parent record if child records exist\n")
	md.WriteString("- **SET_NULL**: Sets foreign key field to NULL when parent record is deleted\n")
	md.WriteString("- **PROTECT**: Same as RESTRICT - prevents deletion of referenced records\n\n")

	// Generate relationship graph if there are many relationships
	if len(relationships) > 3 {
		md.WriteString("### Relationship Diagram\n\n")
		md.WriteString("```mermaid\n")
		md.WriteString("graph TD\n")

		// Create nodes for each table
		tables := make(map[string]bool)
		for _, rel := range relationships {
			tables[rel.FromTable] = true
			tables[rel.ToTable] = true
		}

		// Add relationship arrows
		for _, rel := range relationships {
			arrow := "-->"
			label := rel.FromField
			if rel.OnDelete == "CASCADE" {
				arrow = "==>"
				label += " (CASCADE)"
			}

			fmt.Fprintf(md, "    %s %s|%s| %s\n",
				strings.ToUpper(rel.ToTable), arrow, label, strings.ToUpper(rel.FromTable))
		}

		md.WriteString("```\n\n")
		md.WriteString("*This diagram shows the direction of foreign key relationships. ")
		md.WriteString("Thick arrows (==>) indicate CASCADE relationships.*\n\n")
	}
}

// Helper functions for generating documentation

func convertTypeForMermaid(fieldType string) string {
	switch fieldType {
	case "varchar", "text":
		return "string"
	case "integer", "bigint", "serial":
		return "int"
	case "decimal", "float":
		return "decimal"
	case "boolean":
		return "boolean"
	case "timestamp", "date", "time":
		return "datetime"
	case "uuid":
		return "uuid"
	case "json", "jsonb":
		return "json"
	case "foreign_key":
		return "int"
	default:
		return "string"
	}
}

func generateFieldConstraints(field types.Field) string {
	var constraints []string

	if field.PrimaryKey {
		constraints = append(constraints, "PK")
	}

	if field.Type == "foreign_key" {
		constraints = append(constraints, "FK")
	}

	if !field.IsNullable() {
		constraints = append(constraints, "NOT_NULL")
	}

	if len(constraints) == 0 {
		return ""
	}

	return "\"" + strings.Join(constraints, ",") + "\""
}

func generateRelationshipType(onDelete string) string {
	switch onDelete {
	case "CASCADE":
		return "||--o{"
	case "RESTRICT", "PROTECT":
		return "||--||"
	case "SET_NULL":
		return "||--o|"
	default:
		return "||--||"
	}
}

func generateFieldTypeDescription(field types.Field) string {
	switch field.Type {
	case "varchar":
		if field.Length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", field.Length)
		}
		return "VARCHAR"
	case "text":
		if field.Length > 0 {
			return fmt.Sprintf("TEXT(%d)", field.Length)
		}
		return "TEXT"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", field.Precision, field.Scale)
		}
		return "DECIMAL"
	case "foreign_key":
		if field.ForeignKey != nil {
			return fmt.Sprintf("FK → %s", field.ForeignKey.Table)
		}
		return "FOREIGN KEY"
	default:
		return strings.ToUpper(field.Type)
	}
}

func generateFieldConstraintsDescription(field types.Field) string {
	var constraints []string

	if field.PrimaryKey {
		constraints = append(constraints, "PRIMARY KEY")
	}

	if !field.IsNullable() {
		constraints = append(constraints, "NOT NULL")
	} else {
		constraints = append(constraints, "NULLABLE")
	}

	if field.Type == "serial" {
		constraints = append(constraints, "AUTO INCREMENT")
	}

	if len(constraints) == 0 {
		return "-"
	}

	return strings.Join(constraints, ", ")
}

func generateDefaultDescription(field types.Field) string {
	if field.Default == "" {
		return "-"
	}
	return fmt.Sprintf("`%s`", field.Default)
}

func generateFieldDescription(field types.Field) string {
	switch field.Type {
	case "foreign_key":
		if field.ForeignKey != nil {
			return fmt.Sprintf("References %s table", field.ForeignKey.Table)
		}
		return "Foreign key relationship"
	case "serial":
		return "Auto-incrementing primary key"
	case "timestamp":
		if field.AutoCreate {
			return "Automatically set on creation"
		}
		if field.AutoUpdate {
			return "Automatically updated on modification"
		}
		return "Timestamp field"
	case "boolean":
		return "Boolean true/false value"
	case "uuid":
		return "Universally unique identifier"
	case "json", "jsonb":
		return "JSON data with binary storage"
	default:
		return fmt.Sprintf("%s field", cases.Title(language.Und).String(field.Type))
	}
}

func determineRelationshipType(onDelete string) string {
	switch onDelete {
	case "CASCADE":
		return "One-to-many (CASCADE)"
	case "RESTRICT", "PROTECT":
		return "One-to-many (PROTECTED)"
	case "SET_NULL":
		return "One-to-many (OPTIONAL)"
	default:
		return "One-to-many"
	}
}

func init() {
	rootCmd.AddCommand(schema2diagramCmd)

	// Output flags
	schema2diagramCmd.Flags().StringVar(&diagramOutput, "output", "schema-documentation.md",
		"Output Markdown documentation file path")

	// Common flags
	schema2diagramCmd.Flags().BoolVar(&verbose, "verbose", false,
		"Show detailed processing information")
}
