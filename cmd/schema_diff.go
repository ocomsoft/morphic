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
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v3"

	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/workflow"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

var (
	diffVerbose bool
	diffYAML    bool
	diffJSON    bool
)

var diffCmd = &cobra.Command{
	Use:     "schema-diff",
	Aliases: []string{"diff"},
	GroupID: "inspect",
	Short:   "Show schema drift between schema files and migration state",
	Long: `Compare the current schema files against the migration DAG state and show
the differences in both directions:

  In Schema, Not Yet Migrated  — tables/fields/indexes added to schema
  In Migrations, Not in Schema — tables/fields/indexes removed from schema

Default output is a color-coded report with snippets.
Use --yaml for machine-readable YAML output, or --json for JSON.`,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().BoolVar(&diffVerbose, "verbose", false, "Show detailed output")
	diffCmd.Flags().BoolVar(&diffYAML, "yaml", false, "Output as YAML (add/remove/modify sections)")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")
}

func runDiff(_ *cobra.Command, _ []string) error {
	cfg := config.LoadOrDefault(cfgFile)
	migrationsDir := cfg.Migration.Directory

	// 1. Query existing DAG for previous schema state
	var prevSchema *yamlpkg.Schema

	migFiles, err := countMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations directory: %w", err)
	}

	if len(migFiles) > 0 {
		dagOut, err := queryDAG(migrationsDir, diffVerbose)
		if err != nil {
			return fmt.Errorf("querying migration DAG: %w", err)
		}
		prevSchema = schemaStateToYAMLSchema(dagOut.SchemaState, cfg.Database.Type)
	}

	// 2. Parse current schema
	dbType, err := yamlpkg.ParseDatabaseType(cfg.Database.Type)
	if err != nil {
		return fmt.Errorf("invalid database type: %w", err)
	}
	components := workflow.InitializeSchemaComponents(dbType, diffVerbose)
	allSchemas, err := workflow.ScanAndParseSchemas(components, diffVerbose, cfg)
	if err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}

	currentSchema, err := workflow.MergeAndValidateSchemas(components, allSchemas, dbType, diffVerbose)
	if err != nil {
		return fmt.Errorf("merging schemas: %w", err)
	}

	// 3. Diff
	diffEngine := yamlpkg.NewDiffEngine(diffVerbose)
	diff, err := diffEngine.CompareSchemas(prevSchema, currentSchema)
	if err != nil {
		return fmt.Errorf("computing schema diff: %w", err)
	}

	// 4. Output
	switch {
	case diffJSON:
		return formatSchemaDiffJSON(os.Stdout, diff)
	case diffYAML:
		return formatSchemaDiffYAML(os.Stdout, diff)
	default:
		formatSchemaDiffReport(os.Stdout, diff, diffVerbose)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Human-readable report (default)
// ---------------------------------------------------------------------------

func formatSchemaDiffReport(w io.Writer, diff *yamlpkg.SchemaDiff, verboseFlag bool) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	_, _ = bold.Fprintln(w, "Schema Diff Report")
	_, _ = bold.Fprintln(w, "==================")
	_, _ = fmt.Fprintln(w)

	if !diff.HasChanges {
		_, _ = green.Fprintln(w, "No differences — schema.yaml matches migration state.")
		return
	}

	// Partition changes
	var schemaOnlyTables []yamlpkg.Change // table_added: in schema, not migrated
	var migOnlyTables []yamlpkg.Change    // table_removed: in migrations, not in schema
	fieldChanges := make(map[string][]yamlpkg.Change)
	indexChanges := make(map[string][]yamlpkg.Change)
	fkChanges := make(map[string][]yamlpkg.Change)
	var modifiedFields []yamlpkg.Change
	totalFieldChanges := 0
	totalIndexChanges := 0
	totalFKChanges := 0

	for _, ch := range diff.Changes {
		switch {
		case ch.Type == yamlpkg.ChangeTypeTableAdded:
			schemaOnlyTables = append(schemaOnlyTables, ch)
		case ch.Type == yamlpkg.ChangeTypeTableRemoved:
			migOnlyTables = append(migOnlyTables, ch)
		case ch.Type == yamlpkg.ChangeTypeFieldModified:
			modifiedFields = append(modifiedFields, ch)
		case ch.Type == yamlpkg.ChangeTypeIndexAdded || ch.Type == yamlpkg.ChangeTypeIndexRemoved:
			indexChanges[ch.TableName] = append(indexChanges[ch.TableName], ch)
			totalIndexChanges++
		case isForeignKeyChange(ch):
			fkChanges[ch.TableName] = append(fkChanges[ch.TableName], ch)
			totalFKChanges++
		default:
			fieldChanges[ch.TableName] = append(fieldChanges[ch.TableName], ch)
			totalFieldChanges++
		}
	}

	// --- In Schema, Not Yet Migrated ---
	if len(schemaOnlyTables) > 0 {
		_, _ = bold.Fprintf(w, "In Schema, Not Yet Migrated (%d table(s)):\n", len(schemaOnlyTables))
		for _, ch := range schemaOnlyTables {
			_, _ = yellow.Fprintf(w, "  + %s\n", ch.TableName)
			if verboseFlag {
				_, _ = fmt.Fprintf(w, "    %s\n", ch.Description)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- In Migrations, Removed from Schema ---
	if len(migOnlyTables) > 0 {
		_, _ = bold.Fprintf(w, "In Migrations, Removed from Schema (%d table(s)):\n", len(migOnlyTables))
		for _, ch := range migOnlyTables {
			_, _ = red.Fprintf(w, "  ✗ %s\n", ch.TableName)
			if verboseFlag {
				_, _ = fmt.Fprintf(w, "    %s\n", ch.Description)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- Field Differences ---
	if len(fieldChanges) > 0 {
		tableNames := sortedKeys(fieldChanges)
		_, _ = bold.Fprintf(w, "Field Differences (%d change(s) across %d table(s)):\n", totalFieldChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range fieldChanges[tableName] {
				switch ch.Type {
				case yamlpkg.ChangeTypeFieldAdded:
					_, _ = yellow.Fprintf(w, "    + %s\n", ch.FieldName)
				case yamlpkg.ChangeTypeFieldRemoved:
					_, _ = red.Fprintf(w, "    ✗ %s\n", ch.FieldName)
				default:
					_, _ = fmt.Fprintf(w, "    ~ %s: %s\n", ch.FieldName, ch.Description)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- Modified Fields ---
	if len(modifiedFields) > 0 {
		_, _ = bold.Fprintf(w, "Modified Fields (%d):\n", len(modifiedFields))
		for _, ch := range modifiedFields {
			_, _ = cyan.Fprintf(w, "  %s.%s:\n", ch.TableName, ch.FieldName)
			_, _ = fmt.Fprintf(w, "    %s\n", ch.Description)
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- Index Differences ---
	if len(indexChanges) > 0 {
		tableNames := sortedKeys(indexChanges)
		_, _ = bold.Fprintf(w, "Index Differences (%d change(s) across %d table(s)):\n", totalIndexChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range indexChanges[tableName] {
				switch ch.Type {
				case yamlpkg.ChangeTypeIndexAdded:
					_, _ = yellow.Fprintf(w, "    + %s\n", ch.FieldName)
				case yamlpkg.ChangeTypeIndexRemoved:
					_, _ = red.Fprintf(w, "    ✗ %s\n", ch.FieldName)
				}
				if verboseFlag {
					_, _ = fmt.Fprintf(w, "      %s\n", ch.Description)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- Foreign Key Differences ---
	if len(fkChanges) > 0 {
		tableNames := sortedKeys(fkChanges)
		_, _ = bold.Fprintf(w, "Foreign Key Differences (%d change(s) across %d table(s)):\n", totalFKChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range fkChanges[tableName] {
				switch ch.Type {
				case yamlpkg.ChangeTypeForeignKeyAdded:
					_, _ = yellow.Fprintf(w, "    + %s\n", ch.FieldName)
				case yamlpkg.ChangeTypeForeignKeyRemoved:
					_, _ = red.Fprintf(w, "    ✗ %s\n", ch.FieldName)
				default:
					_, _ = fmt.Fprintf(w, "    ~ %s: %s\n", ch.FieldName, ch.Description)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// --- Summary ---
	_, _ = bold.Fprintln(w, "Summary:")
	_, _ = fmt.Fprintf(w, "  Total differences:    %d\n", len(diff.Changes))
	_, _ = fmt.Fprintf(w, "  Schema-only tables:   %d\n", len(schemaOnlyTables))
	_, _ = fmt.Fprintf(w, "  Migration-only tables: %d\n", len(migOnlyTables))
	_, _ = fmt.Fprintf(w, "  Field changes:        %d\n", totalFieldChanges+len(modifiedFields))
	_, _ = fmt.Fprintf(w, "  Index changes:        %d\n", totalIndexChanges)
	_, _ = fmt.Fprintf(w, "  FK changes:           %d\n", totalFKChanges)

	if diff.IsDestructive {
		_, _ = fmt.Fprintln(w)
		_, _ = red.Fprintln(w, "⚠ One or more differences are destructive (data loss risk).")
	}

	// --- YAML Snippets ---
	_, _ = fmt.Fprintln(w)
	_, _ = bold.Fprintln(w, "YAML Snippets")
	_, _ = bold.Fprintln(w, "=============")

	snippetCount := 0

	// Tables in schema but not migrated
	for _, ch := range schemaOnlyTables {
		table, ok := ch.NewValue.(yamlpkg.Table)
		if !ok {
			continue
		}
		snippet, err := generateTableSnippet(table)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintln(w)
		_, _ = cyan.Fprintf(w, "# Table '%s' (in schema, not yet migrated):\n", ch.TableName)
		_, _ = fmt.Fprint(w, snippet)
		snippetCount++
	}

	// Tables in migrations but removed from schema
	for _, ch := range migOnlyTables {
		table, ok := ch.OldValue.(yamlpkg.Table)
		if !ok {
			continue
		}
		snippet, err := generateTableSnippet(table)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintln(w)
		_, _ = cyan.Fprintf(w, "# Table '%s' (in migrations, removed from schema):\n", ch.TableName)
		_, _ = fmt.Fprint(w, snippet)
		snippetCount++
	}

	// Added fields
	for _, tableName := range sortedKeys(fieldChanges) {
		for _, ch := range fieldChanges[tableName] {
			if ch.Type != yamlpkg.ChangeTypeFieldAdded {
				continue
			}
			field, ok := ch.NewValue.(yamlpkg.Field)
			if !ok {
				continue
			}
			snippet, err := generateFieldSnippet(tableName, field)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintln(w)
			_, _ = cyan.Fprintf(w, "# Add field '%s' to table '%s':\n", ch.FieldName, tableName)
			_, _ = fmt.Fprint(w, snippet)
			snippetCount++
		}
	}

	// Removed fields
	for _, tableName := range sortedKeys(fieldChanges) {
		for _, ch := range fieldChanges[tableName] {
			if ch.Type != yamlpkg.ChangeTypeFieldRemoved {
				continue
			}
			field, ok := ch.OldValue.(yamlpkg.Field)
			if !ok {
				continue
			}
			snippet, err := generateFieldSnippet(tableName, field)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintln(w)
			_, _ = cyan.Fprintf(w, "# Field '%s' on table '%s' (in migrations, removed from schema):\n", ch.FieldName, tableName)
			_, _ = fmt.Fprint(w, snippet)
			snippetCount++
		}
	}

	if snippetCount == 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "(no snippets to show)")
	}
}

// ---------------------------------------------------------------------------
// YAML output (--yaml)
// ---------------------------------------------------------------------------

func formatSchemaDiffYAML(w io.Writer, diff *yamlpkg.SchemaDiff) error {
	if !diff.HasChanges {
		_, _ = fmt.Fprintln(w, "# No differences — schema.yaml matches migration state.")
		return nil
	}

	output := buildDiffYAML(diff.Changes)

	data, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshalling diff output: %w", err)
	}

	_, _ = fmt.Fprintf(w, "# Schema diff: %d change(s) between schema and migration state\n", len(diff.Changes))
	_, _ = fmt.Fprintln(w, "# 'add' = in schema.yaml but not yet migrated")
	_, _ = fmt.Fprintln(w, "# 'remove' = in migration state but removed from schema.yaml")
	_, _ = fmt.Fprintln(w, "# 'modify' = field properties changed")
	_, _ = fmt.Fprint(w, string(data))

	return nil
}

// diffOutput is the top-level YAML structure for the diff command.
type diffOutput struct {
	Add    *diffSection   `yaml:"add,omitempty"`
	Remove *diffSection   `yaml:"remove,omitempty"`
	Modify []diffModified `yaml:"modify,omitempty"`
}

// diffSection holds tables, fields, and indexes for add or remove.
type diffSection struct {
	Tables  []types.Table `yaml:"tables,omitempty"`
	Fields  []diffField   `yaml:"fields,omitempty"`
	Indexes []diffIndex   `yaml:"indexes,omitempty"`
}

// diffField represents a field change scoped to a table.
type diffField struct {
	Table string      `yaml:"table"`
	Field types.Field `yaml:"field"`
}

// diffIndex represents an index change scoped to a table.
type diffIndex struct {
	Table string      `yaml:"table"`
	Index types.Index `yaml:"index"`
}

// diffModified represents a field whose properties changed.
type diffModified struct {
	Table string      `yaml:"table"`
	Field string      `yaml:"field"`
	From  types.Field `yaml:"from"`
	To    types.Field `yaml:"to"`
}

func buildDiffYAML(changes []yamlpkg.Change) diffOutput {
	var out diffOutput
	var addSection, removeSection diffSection

	for _, c := range changes {
		switch c.Type {
		case yamlpkg.ChangeTypeTableAdded:
			if table, ok := c.NewValue.(yamlpkg.Table); ok {
				addSection.Tables = append(addSection.Tables, table)
			}

		case yamlpkg.ChangeTypeTableRemoved:
			if table, ok := c.OldValue.(yamlpkg.Table); ok {
				removeSection.Tables = append(removeSection.Tables, table)
			}

		case yamlpkg.ChangeTypeFieldAdded:
			if field, ok := c.NewValue.(yamlpkg.Field); ok {
				addSection.Fields = append(addSection.Fields, diffField{
					Table: c.TableName,
					Field: field,
				})
			}

		case yamlpkg.ChangeTypeFieldRemoved:
			if field, ok := c.OldValue.(yamlpkg.Field); ok {
				removeSection.Fields = append(removeSection.Fields, diffField{
					Table: c.TableName,
					Field: field,
				})
			}

		case yamlpkg.ChangeTypeIndexAdded:
			if idx, ok := c.NewValue.(yamlpkg.Index); ok {
				addSection.Indexes = append(addSection.Indexes, diffIndex{
					Table: c.TableName,
					Index: idx,
				})
			}

		case yamlpkg.ChangeTypeIndexRemoved:
			if idx, ok := c.OldValue.(yamlpkg.Index); ok {
				removeSection.Indexes = append(removeSection.Indexes, diffIndex{
					Table: c.TableName,
					Index: idx,
				})
			}

		case yamlpkg.ChangeTypeFieldModified:
			oldField, oldOk := c.OldValue.(yamlpkg.Field)
			newField, newOk := c.NewValue.(yamlpkg.Field)
			if oldOk && newOk {
				out.Modify = append(out.Modify, diffModified{
					Table: c.TableName,
					Field: c.FieldName,
					From:  oldField,
					To:    newField,
				})
			}

		case yamlpkg.ChangeTypeForeignKeyAdded, yamlpkg.ChangeTypeForeignKeyRemoved:
			continue
		}
	}

	if len(addSection.Tables) > 0 || len(addSection.Fields) > 0 || len(addSection.Indexes) > 0 {
		out.Add = &addSection
	}
	if len(removeSection.Tables) > 0 || len(removeSection.Fields) > 0 || len(removeSection.Indexes) > 0 {
		out.Remove = &removeSection
	}

	return out
}

// ---------------------------------------------------------------------------
// JSON output (--json)
// ---------------------------------------------------------------------------

func formatSchemaDiffJSON(w io.Writer, diff *yamlpkg.SchemaDiff) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(diff)
}
