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

// Package workflow provides shared schema processing pipelines used by
// multiple CLI commands (generate, diff, dump-sql, schema2diagram, find-includes).
package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/scanner"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

// SchemaComponents holds the initialized schema processing components
type SchemaComponents struct {
	StateManager *yamlpkg.StateManager
	Scanner      *scanner.Scanner
	Parser       *yamlpkg.Parser
	Merger       *yamlpkg.Merger
	DiffEngine   *yamlpkg.DiffEngine
}

// InitializeSchemaComponents creates and initializes all schema processing components
func InitializeSchemaComponents(dbType yamlpkg.DatabaseType, verbose bool) *SchemaComponents {
	return &SchemaComponents{
		StateManager: yamlpkg.NewStateManager(verbose),
		Scanner:      scanner.New(verbose),
		Parser:       yamlpkg.NewParser(verbose),
		Merger:       yamlpkg.NewMerger(verbose),
		DiffEngine:   yamlpkg.NewDiffEngine(verbose),
	}
}

// ScanAndParseSchemas scans for Starlark schema files and loads them
func ScanAndParseSchemas(components *SchemaComponents, verbose bool) ([]*yamlpkg.Schema, error) {
	// Scan for Starlark schema files
	schemaFiles, err := components.Scanner.ScanStarlarkModules()
	if err != nil {
		return nil, fmt.Errorf("failed to scan modules: %w", err)
	}

	if verbose {
		color.Green("Found %d schema files\n", len(schemaFiles))
		for _, file := range schemaFiles {
			marker := ""
			if file.HasMarker {
				marker = " (with marker)"
			}
			color.Cyan("  - %s%s\n", file.ModulePath, marker)
		}
	}

	if len(schemaFiles) == 0 {
		return nil, fmt.Errorf("no schema files found")
	}

	// Load all schemas using interp.LoadSchema
	var allSchemas []*yamlpkg.Schema
	for _, file := range schemaFiles {
		if verbose {
			color.Blue("Processing schema file: %s\n", file.ModulePath)
		}

		// Use interp.LoadSchema on the parent directory of the discovered file
		schemaDir := filepath.Dir(file.FilePath)
		schema, err := interp.LoadSchema(schemaDir, verbose)
		if err != nil {
			return nil, fmt.Errorf("loading schema failed for %s: %w", file.ModulePath, err)
		}

		// Run basic structure validation but continue if it fails
		if err := components.Parser.ValidateSchemaStructure(schema); err != nil {
			color.Yellow("Structure validation warning for %s: %v\n", file.ModulePath, err)
		}

		allSchemas = append(allSchemas, schema)
	}

	return allSchemas, nil
}

// MergeAndValidateSchemas merges schemas and validates the result
func MergeAndValidateSchemas(components *SchemaComponents, allSchemas []*yamlpkg.Schema, dbType yamlpkg.DatabaseType, verbose bool) (*yamlpkg.Schema, error) {
	// Merge schemas
	mergedSchema, err := components.Merger.MergeSchemas(allSchemas)
	if err != nil {
		return nil, fmt.Errorf("failed to merge schemas: %w", err)
	}

	if verbose {
		color.Green("Merged schema: %d tables\n", len(mergedSchema.Tables))
		color.Blue("Available tables:")
		for _, table := range mergedSchema.Tables {
			color.Cyan("  - %s\n", table.Name)
		}
	}

	// Final validation on merged schema - show issues but continue
	finalValidationErrors := components.Parser.ValidateComprehensive(mergedSchema, dbType)
	if len(finalValidationErrors) > 0 {
		color.Yellow("\nMerged schema validation issues:\n")
		fmt.Print(components.Parser.FormatValidationErrors(finalValidationErrors))

		// Check if there are fatal errors that prevent migration generation
		hasFatalErrors := false
		for _, validationErr := range finalValidationErrors {
			if validationErr.Severity != "warning" {
				hasFatalErrors = true
				break
			}
		}

		if hasFatalErrors {
			return nil, fmt.Errorf("merged schema validation failed - please fix the foreign key references and other errors")
		}
	}

	return mergedSchema, nil
}

// DAGQuerier provides the callback functions needed by ExecuteDumpSQL to query
// the migration DAG without depending on the migrate or interp packages.
type DAGQuerier struct {
	// QueryDAG loads migrations from the given directory and returns the
	// previous schema as a *yamlpkg.Schema (or nil if no migrations exist).
	QueryPreviousSchema func(migrationsDir string, dbType string, verbose bool) (*yamlpkg.Schema, error)
}

// ExecuteDumpSQL handles the complete dump SQL process.
// When pending is true, it shows only the SQL for pending schema changes
// (what the next migration would do). When false, it dumps the full schema.
func ExecuteDumpSQL(cmd *cobra.Command, databaseType string, pending bool, verbose bool, configFile string, dagQuerier *DAGQuerier) error {
	// Parse database type
	dbType, err := yamlpkg.ParseDatabaseType(databaseType)
	if err != nil {
		return fmt.Errorf("invalid database type: %w", err)
	}

	// Initialize schema components
	components := InitializeSchemaComponents(dbType, verbose)

	if verbose {
		if pending {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Dumping pending schema changes as SQL\n")
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "=====================================\n")
		} else {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Dumping merged schema as SQL\n")
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "============================\n")
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Database type: %s\n", dbType)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n1. Scanning Go modules for schema files...\n")
	}

	// Scan and parse schemas
	allSchemas, err := ScanAndParseSchemas(components, false)
	if err != nil {
		if err.Error() == "no schema files found" {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "No schema files found. Nothing to dump.\n")
			return nil
		}
		return err
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n2. Parsing and merging schemas...\n")
	}

	// Merge and validate schemas
	mergedSchema, err := MergeAndValidateSchemas(components, allSchemas, dbType, false)
	if err != nil {
		return fmt.Errorf("merged schema validation failed: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Merged schema: %d tables\n", len(mergedSchema.Tables))
	}

	if pending {
		return executePendingDumpSQL(cmd, dbType, mergedSchema, verbose, configFile, dagQuerier)
	}

	return executeFullDumpSQL(cmd, dbType, mergedSchema, verbose)
}

// executeFullDumpSQL dumps the complete schema as CREATE TABLE statements.
func executeFullDumpSQL(cmd *cobra.Command, dbType yamlpkg.DatabaseType, mergedSchema *yamlpkg.Schema, verbose bool) error {
	sqlConverter := yamlpkg.NewSQLConverter(dbType, verbose)

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n3. Generating SQL...\n")
	}

	sql, err := sqlConverter.ConvertSchema(mergedSchema)
	if err != nil {
		return fmt.Errorf("failed to generate SQL: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Generated %d lines of SQL\n", len(sql))
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nSQL Output:\n")
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "================\n")
	}

	fmt.Print(sql)
	return nil
}

// executePendingDumpSQL shows only SQL for pending schema changes by comparing
// the current YAML schema against the state from existing migrations.
func executePendingDumpSQL(cmd *cobra.Command, dbType yamlpkg.DatabaseType, currentSchema *yamlpkg.Schema, verbose bool, configFile string, dagQuerier *DAGQuerier) error {
	cfg := config.LoadOrDefault(configFile)
	migrationsDir := cfg.Migration.Directory

	// Check for existing migration files
	var prevSchema *yamlpkg.Schema

	goFiles, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	if err != nil {
		return fmt.Errorf("scanning migrations directory: %w", err)
	}

	// Filter to migration files only (exclude main.go)
	var migFiles []string
	for _, f := range goFiles {
		if filepath.Base(f) != "main.go" {
			migFiles = append(migFiles, f)
		}
	}

	if len(migFiles) > 0 {
		if verbose {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n3. Querying migration DAG for previous state...\n")
		}
		prevSchema, err = dagQuerier.QueryPreviousSchema(migrationsDir, string(dbType), verbose)
		if err != nil {
			return fmt.Errorf("querying migration DAG: %w", err)
		}
	} else {
		if verbose {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n3. No existing migrations found, treating previous state as empty...\n")
		}
	}

	// Diff previous state against current schema
	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n4. Computing schema diff...\n")
	}
	diffEngine := yamlpkg.NewDiffEngine(verbose)
	diff, err := diffEngine.CompareSchemas(prevSchema, currentSchema)
	if err != nil {
		return fmt.Errorf("computing schema diff: %w", err)
	}

	if !diff.HasChanges {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No pending changes.")
		return nil
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Found %d pending changes\n", len(diff.Changes))
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n5. Generating SQL for pending changes...\n")
	}

	// Convert diff to SQL using silent mode (no interactive prompts)
	sqlConverter := yamlpkg.NewSQLConverterWithConfig(dbType, verbose, false, "", nil, true, "")
	upSQL, _, err := sqlConverter.ConvertDiffToSQL(diff, prevSchema, currentSchema)
	if err != nil {
		return fmt.Errorf("failed to generate pending SQL: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nPending SQL Output:\n")
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "================\n")
	}

	fmt.Print(upSQL)
	return nil
}

// DiscoveredSchema represents a schema found during discovery
type DiscoveredSchema struct {
	ModulePath   string
	RelativePath string
	FullPath     string
	IsWorkspace  bool
	Schema       *yamlpkg.Schema
	TableCount   int
	DatabaseName string
	DatabaseType string
}

// LocalSchemaFile represents a local schema file found in the current directory
type LocalSchemaFile struct {
	Path         string
	DatabaseName string
	TableCount   int
}

// FindIncludesCallbacks holds the callback functions needed by ExecuteFindIncludes
// to interact with the file system and user without depending on cmd-internal types.
type FindIncludesCallbacks struct {
	FindLocalSchemaFiles       func() ([]LocalSchemaFile, error)
	SelectLocalSchemaFile      func([]LocalSchemaFile) (string, error)
	DiscoverSchemas            func() ([]DiscoveredSchema, error)
	SelectSchemasInteractively func([]DiscoveredSchema) ([]DiscoveredSchema, error)
	LoadExistingSchema         func(string) (*yamlpkg.Schema, error)
	FilterNewSchemas           func([]DiscoveredSchema, *yamlpkg.Schema) []DiscoveredSchema
	UpdateSchemaWithIncludes   func(string, *yamlpkg.Schema, []DiscoveredSchema) error
}

// ExecuteFindIncludes handles the complete find includes process
func ExecuteFindIncludes(cmd *cobra.Command, configFile, schemaPath string, interactive, includeWorkspace bool, callbacks *FindIncludesCallbacks) error {
	// Load configuration
	cfg := config.LoadOrDefault(configFile)

	// Apply config settings
	verbose := cfg.Output.Verbose
	if !cfg.Output.ColorEnabled {
		color.NoColor = true
	}

	if verbose {
		color.Cyan("Schema Include Discovery Tool")
		color.Cyan("=============================")
	}

	// Check if schema flag was provided
	schemaProvided := cmd.Flags().Changed("schema")

	// If schema not provided, search for schema.yaml files
	if !schemaProvided {
		if verbose {
			color.Blue("No --schema flag provided, searching for schema files...")
		}

		localSchemas, err := callbacks.FindLocalSchemaFiles()
		if err != nil {
			return fmt.Errorf("failed to search for local schema files: %w", err)
		}

		if len(localSchemas) == 0 {
			return fmt.Errorf("no schema files found in current directory and subdirectories")
		}

		if len(localSchemas) == 1 {
			// Use the single schema file found
			schemaPath = localSchemas[0].Path
			if verbose {
				color.Green("Found schema file: %s (database: %s)", schemaPath, localSchemas[0].DatabaseName)
			}
		} else {
			// Multiple schema files found, prompt user
			selectedPath, err := callbacks.SelectLocalSchemaFile(localSchemas)
			if err != nil {
				return fmt.Errorf("failed to select schema file: %w", err)
			}
			schemaPath = selectedPath
		}
	}

	// Validate schema file exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		return fmt.Errorf("schema file not found: %s", schemaPath)
	}

	if verbose {
		color.Blue("\n1. Discovering schemas in Go modules...")
	}

	// Discover schemas
	discovered, err := callbacks.DiscoverSchemas()
	if err != nil {
		return fmt.Errorf("failed to discover schemas: %w", err)
	}

	if len(discovered) == 0 {
		color.Yellow("No schemas found in Go modules.")
		return nil
	}

	if verbose {
		color.Green("Found %d schema(s)\n", len(discovered))
	}

	// Load existing schema
	existingSchema, err := callbacks.LoadExistingSchema(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to load existing schema: %w", err)
	}

	// Filter out already included schemas
	newSchemas := callbacks.FilterNewSchemas(discovered, existingSchema)
	if len(newSchemas) == 0 {
		color.Yellow("All discovered schemas are already included.")
		return nil
	}

	if verbose {
		color.Blue("\n2. Processing discovered schemas...")
	}

	// Handle interactive vs automatic mode
	var schemasToAdd []DiscoveredSchema
	if interactive {
		schemasToAdd, err = callbacks.SelectSchemasInteractively(newSchemas)
		if err != nil {
			return fmt.Errorf("interactive selection failed: %w", err)
		}
	} else {
		schemasToAdd = newSchemas
		if verbose {
			color.Green("Adding %d new schema(s) automatically\n", len(schemasToAdd))
		}
	}

	if len(schemasToAdd) == 0 {
		color.Yellow("No schemas selected for inclusion.")
		return nil
	}

	if verbose {
		color.Blue("\n3. Updating schema file...")
	}

	// Update schema file
	err = callbacks.UpdateSchemaWithIncludes(schemaPath, existingSchema, schemasToAdd)
	if err != nil {
		return fmt.Errorf("failed to update schema: %w", err)
	}

	color.Green("\nSuccessfully added %d include(s) to %s", len(schemasToAdd), schemaPath)

	// Show what was added
	color.Cyan("\nAdded includes:")
	for _, schema := range schemasToAdd {
		marker := ""
		if schema.IsWorkspace {
			marker = " (workspace)"
		}
		color.Cyan("  - %s -> %s%s", schema.ModulePath, schema.RelativePath, marker)
	}

	return nil
}
