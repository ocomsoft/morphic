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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v3"

	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/providers"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

// dbDiffFormat controls the output format for the db-diff command ("text" or "json").
var dbDiffFormat string

// dbDiffCmd represents the db-diff command that compares a live database schema
// against the schema state reconstructed from the migration DAG.
var dbDiffCmd = &cobra.Command{
	Use:     "db-diff",
	GroupID: "inspect",
	Short:   "Compare live database schema against migration DAG state",
	Long: `Compare the live database schema against the expected schema derived from the
migration DAG (Directed Acyclic Graph).

The command connects to a live database, introspects its schema, and compares it
against the schema state reconstructed by compiling and querying the Go migration
files. Differences are reported in five categories:

  1. Missing from DB      — tables defined in migrations but absent from the
     live database (may indicate unapplied migrations).
  2. Extra in DB          — tables present in the database but not tracked by
     any migration (may indicate manual DDL or external tools).
  3. Field Differences    — columns that exist in both but differ in type, length,
     nullable, default, or other properties.
  4. Index Differences    — indexes that differ between the migration state and
     the live database (added, removed, or modified).
  5. Foreign Key Differences — foreign key constraints that differ in referenced
     table or ON DELETE action.

When differences are found, pasteable YAML definitions are appended to the text
output for each difference. These snippets can be copied directly into a schema
YAML file.

Database Connection:
  Use individual flags (--host, --port, --database, --username, --password,
  --sslmode) or rely on the config file (migrations/morphic.config.yaml).
  Command-line flags take precedence over config file settings.

Examples:
  # Compare using connection flags (text output)
  morphic db-diff --host=localhost --port=5432 --database=myapp --username=user

  # Compare with JSON output
  morphic db-diff --host=localhost --database=myapp --format=json

  # Compare using config file settings
  morphic db-diff --config=migrations/morphic.config.yaml

  # Compare with verbose output
  morphic db-diff --verbose --host=localhost --database=myapp`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDBDiff(cmd)
	},
}

// runDBDiff orchestrates the db-diff workflow: loading the DAG schema from
// compiled migrations and the live database schema, then comparing them.
func runDBDiff(cmd *cobra.Command) error {
	// Load config to determine the migrations directory
	cfg := config.LoadOrDefault(cfgFile)
	migrationsDir := cfg.Migration.Directory

	// Parse the database type from the --db-type flag
	dbType, err := yamlpkg.ParseDatabaseType(databaseType)
	if err != nil {
		return fmt.Errorf("invalid database type: %w", err)
	}

	// Only PostgreSQL has a full GetDatabaseSchema implementation. All other
	// providers return a "not implemented yet" error. Give a clear message
	// upfront rather than letting the provider return a cryptic stub error.
	if dbType != yamlpkg.DatabasePostgreSQL {
		return fmt.Errorf(
			"db-diff does not yet support %q — only postgresql is fully implemented.\n"+
				"To add support, implement GetDatabaseSchema in internal/providers/%s/provider.go",
			dbType, dbType,
		)
	}

	// Load DAG schema by checking for Go migration files
	var dagSchema *yamlpkg.Schema

	// filepath.Glob only errors on malformed patterns; "*.go" is always valid.
	goFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.go"))

	// Filter out main.go from the migration file list
	var migrationFiles []string
	for _, f := range goFiles {
		if filepath.Base(f) != "main.go" {
			migrationFiles = append(migrationFiles, f)
		}
	}

	if len(migrationFiles) > 0 {
		dagOutput, dagErr := queryDAG(migrationsDir, verbose)
		if dagErr != nil {
			return fmt.Errorf("failed to query migration DAG: %w", dagErr)
		}
		dagSchema = schemaStateToYAMLSchema(dagOutput.SchemaState, string(dbType))
	} else {
		dagSchema = &yamlpkg.Schema{}
	}

	// Load the live database schema via the provider
	provider, err := providers.NewProvider(dbType, nil)
	if err != nil {
		return fmt.Errorf("failed to create database provider: %w", err)
	}

	connectionString := buildConnectionString(dbType, cfg.Database.DefaultURL)
	dbSchema, err := provider.GetDatabaseSchema(connectionString)
	if err != nil {
		return fmt.Errorf("failed to get database schema: %w", err)
	}

	return runDBDiffWithSchemas(cmd.OutOrStdout(), dagSchema, dbSchema, dbDiffFormat, verbose)
}

// runDBDiffWithSchemas compares two schemas and writes the result to w. This
// function is separated from runDBDiff to allow unit testing without a live
// database connection.
func runDBDiffWithSchemas(w io.Writer, dagSchema, dbSchema *yamlpkg.Schema, format string, verboseFlag bool) error {
	normalizeDBSchema(dbSchema)

	diffEngine := yamlpkg.NewDiffEngine(verboseFlag)
	// DAG schema is "old"; live DB schema is "new".
	// ChangeTypeTableRemoved => table in DAG but missing from DB (unapplied migration).
	// ChangeTypeTableAdded   => table in DB but not in DAG (manual DDL or external tool).
	diff, err := diffEngine.CompareSchemas(dagSchema, dbSchema)
	if err != nil {
		return fmt.Errorf("failed to compare schemas: %w", err)
	}

	if format == "json" {
		return formatDBDiffJSON(w, diff)
	}

	var buf bytes.Buffer
	formatDBDiff(&buf, diff, verboseFlag)
	_, _ = fmt.Fprint(w, buf.String())

	if diff.HasChanges {
		return fmt.Errorf("schema drift detected: %d difference(s) found", len(diff.Changes))
	}

	return nil
}

// sqlTypeMapping maps SQL-native types (lowercase) returned by database
// introspection to the canonical YAML schema types used by morphic.
var sqlTypeMapping = map[string]string{
	"character varying":           "varchar",
	"character":                   "varchar",
	"char":                        "varchar",
	"int":                         "integer",
	"int2":                        "integer",
	"int4":                        "integer",
	"smallint":                    "integer",
	"int8":                        "bigint",
	"float4":                      "float",
	"float8":                      "float",
	"double precision":            "float",
	"real":                        "float",
	"numeric":                     "decimal",
	"bool":                        "boolean",
	"timestamp without time zone": "timestamp",
	"timestamp with time zone":    "timestamp",
	"timestamptz":                 "timestamp",
	"serial4":                     "serial",
	"serial8":                     "serial",
}

// normalizeDBSchema maps SQL-native column types in the schema to canonical
// YAML types. Unknown types are left unchanged. The function is idempotent.
func normalizeDBSchema(schema *yamlpkg.Schema) {
	if schema == nil {
		return
	}
	for i := range schema.Tables {
		for j := range schema.Tables[i].Fields {
			lower := strings.ToLower(schema.Tables[i].Fields[j].Type)
			if mapped, ok := sqlTypeMapping[lower]; ok {
				schema.Tables[i].Fields[j].Type = mapped
			}
		}
	}
}

// formatDBDiff writes a human-readable, color-coded diff report to w.
func formatDBDiff(w io.Writer, diff *yamlpkg.SchemaDiff, verboseFlag bool) {
	bold := color.New(color.Bold)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)
	green := color.New(color.FgGreen)

	// Header
	_, _ = bold.Fprintln(w, "DB-Diff Report")
	_, _ = bold.Fprintln(w, "==============")
	_, _ = fmt.Fprintln(w)

	// No changes — early return
	if !diff.HasChanges {
		_, _ = green.Fprintln(w, "No differences found. The live database matches the migration DAG schema.")
		return
	}

	// Partition changes into categories
	var missingTables []yamlpkg.Change
	var extraTables []yamlpkg.Change
	fieldChanges := make(map[string][]yamlpkg.Change)
	indexChanges := make(map[string][]yamlpkg.Change)
	fkChanges := make(map[string][]yamlpkg.Change)
	totalFieldChanges := 0
	totalIndexChanges := 0
	totalFKChanges := 0

	for _, ch := range diff.Changes {
		switch {
		case ch.Type == yamlpkg.ChangeTypeTableRemoved:
			missingTables = append(missingTables, ch)
		case ch.Type == yamlpkg.ChangeTypeTableAdded:
			extraTables = append(extraTables, ch)
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

	// Missing from DB section
	if len(missingTables) > 0 {
		_, _ = bold.Fprintf(w, "Missing from DB (%d):\n", len(missingTables))
		for _, ch := range missingTables {
			_, _ = red.Fprintf(w, "  %s %s\n", "\u2717", ch.TableName)
			if verboseFlag {
				_, _ = fmt.Fprintf(w, "    %s\n", ch.Description)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Extra in DB section
	if len(extraTables) > 0 {
		_, _ = bold.Fprintf(w, "Extra in DB (%d):\n", len(extraTables))
		for _, ch := range extraTables {
			_, _ = yellow.Fprintf(w, "  + %s\n", ch.TableName)
			if verboseFlag {
				_, _ = fmt.Fprintf(w, "    %s\n", ch.Description)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Field Differences section
	if len(fieldChanges) > 0 {
		tableNames := sortedKeys(fieldChanges)
		_, _ = bold.Fprintf(w, "Field Differences (%d change(s) across %d table(s)):\n", totalFieldChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range fieldChanges[tableName] {
				switch ch.Type {
				case yamlpkg.ChangeTypeFieldRemoved:
					_, _ = red.Fprintf(w, "    %s %s\n", "\u2717", ch.FieldName)
				case yamlpkg.ChangeTypeFieldAdded:
					_, _ = yellow.Fprintf(w, "    + %s\n", ch.FieldName)
				default:
					_, _ = fmt.Fprintf(w, "    ~ %s: %s\n", ch.FieldName, ch.Description)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Index Differences section
	if len(indexChanges) > 0 {
		tableNames := sortedKeys(indexChanges)
		_, _ = bold.Fprintf(w, "Index Differences (%d change(s) across %d table(s)):\n", totalIndexChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range indexChanges[tableName] {
				switch ch.Type {
				case yamlpkg.ChangeTypeIndexRemoved:
					_, _ = red.Fprintf(w, "    %s %s\n", "\u2717", ch.FieldName)
				case yamlpkg.ChangeTypeIndexAdded:
					_, _ = yellow.Fprintf(w, "    + %s\n", ch.FieldName)
				}
				if verboseFlag {
					_, _ = fmt.Fprintf(w, "      %s\n", ch.Description)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Foreign Key Differences section
	if len(fkChanges) > 0 {
		tableNames := sortedKeys(fkChanges)
		_, _ = bold.Fprintf(w, "Foreign Key Differences (%d change(s) across %d table(s)):\n", totalFKChanges, len(tableNames))
		for _, tableName := range tableNames {
			_, _ = cyan.Fprintf(w, "  %s:\n", tableName)
			for _, ch := range fkChanges[tableName] {
				_, _ = fmt.Fprintf(w, "    ~ %s: %s\n", ch.FieldName, ch.Description)
			}
		}
		_, _ = fmt.Fprintln(w)
	}

	// Summary section
	_, _ = bold.Fprintln(w, "Summary:")
	_, _ = fmt.Fprintf(w, "  Total differences: %d\n", len(diff.Changes))
	_, _ = fmt.Fprintf(w, "  Missing tables:    %d\n", len(missingTables))
	_, _ = fmt.Fprintf(w, "  Extra tables:      %d\n", len(extraTables))
	_, _ = fmt.Fprintf(w, "  Field changes:     %d\n", totalFieldChanges)
	_, _ = fmt.Fprintf(w, "  Index changes:     %d\n", totalIndexChanges)
	_, _ = fmt.Fprintf(w, "  FK changes:        %d\n", totalFKChanges)

	// Destructive warning
	if diff.IsDestructive {
		_, _ = fmt.Fprintln(w)
		_, _ = red.Fprintln(w, "\u26a0 One or more differences are flagged as destructive (data loss risk).")
	}

	// YAML Snippets section — always appended for text output when changes exist
	_, _ = fmt.Fprintln(w)
	_, _ = bold.Fprintln(w, "YAML Snippets")
	_, _ = bold.Fprintln(w, "=============")

	snippetCount := 0

	// Extra tables — full table YAML for adding to schema
	for _, ch := range extraTables {
		table, ok := ch.NewValue.(yamlpkg.Table)
		if !ok {
			continue
		}
		snippet, err := generateTableSnippet(table)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintln(w)
		_, _ = cyan.Fprintf(w, "# Add table '%s' to your schema:\n", ch.TableName)
		_, _ = fmt.Fprint(w, snippet)
		snippetCount++
	}

	// Missing tables — show the table YAML that exists in DAG for reference
	for _, ch := range missingTables {
		table, ok := ch.OldValue.(yamlpkg.Table)
		if !ok {
			continue
		}
		snippet, err := generateTableSnippet(table)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintln(w)
		_, _ = cyan.Fprintf(w, "# Table '%s' (in DAG, missing from DB — run migrations):\n", ch.TableName)
		_, _ = fmt.Fprint(w, snippet)
		snippetCount++
	}

	// Field changes — show field YAML with table context
	for _, tableName := range sortedKeys(fieldChanges) {
		for _, ch := range fieldChanges[tableName] {
			switch ch.Type {
			case yamlpkg.ChangeTypeFieldAdded:
				field, ok := ch.NewValue.(yamlpkg.Field)
				if !ok {
					continue
				}
				snippet, err := generateFieldSnippet(tableName, field)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintln(w)
				_, _ = cyan.Fprintf(w, "# Extra field '%s.%s' (in DB, not in schema):\n", tableName, ch.FieldName)
				_, _ = fmt.Fprint(w, snippet)
				snippetCount++
			case yamlpkg.ChangeTypeFieldRemoved:
				field, ok := ch.OldValue.(yamlpkg.Field)
				if !ok {
					continue
				}
				snippet, err := generateFieldSnippet(tableName, field)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintln(w)
				_, _ = cyan.Fprintf(w, "# Missing field '%s.%s' (in schema, not in DB):\n", tableName, ch.FieldName)
				_, _ = fmt.Fprint(w, snippet)
				snippetCount++
			}
		}
	}

	// Index changes — show index YAML with table context
	for _, tableName := range sortedKeys(indexChanges) {
		for _, ch := range indexChanges[tableName] {
			switch ch.Type {
			case yamlpkg.ChangeTypeIndexAdded:
				index, ok := ch.NewValue.(yamlpkg.Index)
				if !ok {
					continue
				}
				snippet, err := generateIndexSnippet(tableName, index)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintln(w)
				_, _ = cyan.Fprintf(w, "# Extra index '%s' on '%s' (in DB, not in schema):\n", ch.FieldName, tableName)
				_, _ = fmt.Fprint(w, snippet)
				snippetCount++
			case yamlpkg.ChangeTypeIndexRemoved:
				index, ok := ch.OldValue.(yamlpkg.Index)
				if !ok {
					continue
				}
				snippet, err := generateIndexSnippet(tableName, index)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintln(w)
				_, _ = cyan.Fprintf(w, "# Missing index '%s' on '%s' (in schema, not in DB):\n", ch.FieldName, tableName)
				_, _ = fmt.Fprint(w, snippet)
				snippetCount++
			}
		}
	}

	// FK changes — show field YAML with FK for context
	for _, tableName := range sortedKeys(fkChanges) {
		for _, ch := range fkChanges[tableName] {
			if oldFK, ok := ch.OldValue.(*yamlpkg.ForeignKey); ok {
				snippet, err := generateFieldSnippet(tableName, yamlpkg.Field{
					Name:       ch.FieldName,
					Type:       "foreign_key",
					ForeignKey: oldFK,
				})
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintln(w)
				_, _ = cyan.Fprintf(w, "# FK on '%s.%s' (expected by schema):\n", tableName, ch.FieldName)
				_, _ = fmt.Fprint(w, snippet)
				snippetCount++
			}
		}
	}

	if snippetCount == 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "  No YAML snippets to generate for these changes.")
	}
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string][]yamlpkg.Change) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isForeignKeyChange returns true if the change involves a foreign key modification.
func isForeignKeyChange(ch yamlpkg.Change) bool {
	if ch.Type == yamlpkg.ChangeTypeForeignKeyAdded || ch.Type == yamlpkg.ChangeTypeForeignKeyRemoved {
		return true
	}
	if _, ok := ch.OldValue.(*yamlpkg.ForeignKey); ok {
		return true
	}
	if _, ok := ch.NewValue.(*yamlpkg.ForeignKey); ok {
		return true
	}
	return false
}

// generateTableSnippet marshals a Table to a YAML snippet suitable for pasting
// into a schema file's tables: list.
func generateTableSnippet(table yamlpkg.Table) (string, error) {
	data, err := yaml.Marshal([]yamlpkg.Table{table})
	if err != nil {
		return "", fmt.Errorf("failed to marshal table %s: %w", table.Name, err)
	}
	return string(data), nil
}

// generateFieldSnippet marshals a Field to a YAML snippet with a table-name
// comment for context.
func generateFieldSnippet(tableName string, field yamlpkg.Field) (string, error) {
	data, err := yaml.Marshal([]yamlpkg.Field{field})
	if err != nil {
		return "", fmt.Errorf("failed to marshal field %s.%s: %w", tableName, field.Name, err)
	}
	return fmt.Sprintf("# Table: %s\n%s", tableName, string(data)), nil
}

// generateIndexSnippet marshals an Index to a YAML snippet with a table-name
// comment for context.
func generateIndexSnippet(tableName string, index yamlpkg.Index) (string, error) {
	data, err := yaml.Marshal([]yamlpkg.Index{index})
	if err != nil {
		return "", fmt.Errorf("failed to marshal index %s.%s: %w", tableName, index.Name, err)
	}
	return fmt.Sprintf("# Table: %s\n%s", tableName, string(data)), nil
}

// formatDBDiffJSON writes the SchemaDiff as pretty-printed JSON to w.
func formatDBDiffJSON(w io.Writer, diff *yamlpkg.SchemaDiff) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(diff)
}

func init() {
	rootCmd.AddCommand(dbDiffCmd)

	// Connection flags are bound to package-level variables shared with db2schemaCmd
	// (declared in db2schema.go). This follows the existing project pattern.
	// Each Cobra command maintains its own flag set, so the variables are populated
	// independently per invocation.
	dbDiffCmd.Flags().StringVar(&host, "host", "", "Database host (default: localhost)")
	dbDiffCmd.Flags().IntVar(&port, "port", 0, "Database port")
	dbDiffCmd.Flags().StringVar(&database, "database", "", "Database name")
	dbDiffCmd.Flags().StringVar(&username, "username", "", "Database username")
	dbDiffCmd.Flags().StringVar(&password, "password", "", "Database password")
	dbDiffCmd.Flags().StringVar(&sslmode, "sslmode", "", "SSL mode (default: disable)")

	// Database type flag
	dbDiffCmd.Flags().StringVar(&databaseType, "db-type", "postgresql", "Database type (postgresql, mysql, sqlserver, sqlite)")

	// Output format flag
	dbDiffCmd.Flags().StringVar(&dbDiffFormat, "format", "text", "Output format: text or json")

	// Verbose flag (local to this command, not persistent)
	dbDiffCmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed processing information")
}
