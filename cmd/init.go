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

	"github.com/ocomsoft/morphic/internal/config"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
	"github.com/spf13/cobra"
)

var initDatabaseType string

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:     "init",
	GroupID: "schema",
	Short:   "Initialize migrations directory and config",
	Long: `Bootstrap the migrations/ directory for the morphic migration framework.

This command:
- Creates the migrations/ directory if it doesn't exist
- If a .schema_snapshot.yaml exists, generates an initial 0001_initial.star migration
- Prints instructions for faking the initial migration on an existing database

Use this command when setting up morphic for the first time in a project.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		return executeMigrationInit(initDatabaseType, verbose)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initDatabaseType, "database", "postgresql",
		"Target database type (postgresql, mysql, sqlserver, sqlite)")
	initCmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed processing information")
}

// schemaToInitialDiff converts a yaml.Schema to a SchemaDiff that treats every
// table as newly added.
func schemaToInitialDiff(schema *yamlpkg.Schema, dbType string) *yamlpkg.SchemaDiff {
	diff := &yamlpkg.SchemaDiff{HasChanges: true}

	if defaults := getDefaultsForDB(schema, dbType); len(defaults) > 0 {
		diff.Changes = append(diff.Changes, yamlpkg.Change{
			Type:        yamlpkg.ChangeTypeDefaultsModified,
			Description: "Set initial schema defaults",
			NewValue:    defaults,
		})
	}

	if currMappings := getTypeMappingsForDB(schema, dbType); len(currMappings) > 0 {
		diff.Changes = append(diff.Changes, yamlpkg.Change{
			Type:        yamlpkg.ChangeTypeTypeMappingsModified,
			Description: "Set schema type mappings",
			NewValue:    currMappings,
		})
	}

	for _, t := range schema.Tables {
		diff.Changes = append(diff.Changes, yamlpkg.Change{
			Type:      yamlpkg.ChangeTypeTableAdded,
			TableName: t.Name,
			NewValue:  t,
		})
	}
	return diff
}

// executeMigrationInit initializes the migrations/ directory.
func executeMigrationInit(databaseType string, verbose bool) error {
	cfg := config.DefaultConfig()
	migrationsDir := cfg.Migration.Directory
	gen := newGenerator(cfg)

	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		return fmt.Errorf("creating migrations directory: %w", err)
	}

	var initialMigName string

	sm := yamlpkg.NewStateManager(verbose)
	existingSchema, err := sm.LoadSchemaSnapshot(migrationsDir)
	if err == nil && existingSchema != nil {
		initialMigName = "0001_initial"
		diff := schemaToInitialDiff(existingSchema, databaseType)
		src, genErr := gen.GenerateMigration(initialMigName, []string{}, diff, existingSchema, nil, nil)
		if genErr != nil {
			return fmt.Errorf("generating initial migration: %w", genErr)
		}
		migPath := filepath.Join(migrationsDir, initialMigName+gen.FileExtension())
		if writeErr := os.WriteFile(migPath, []byte(src), 0644); writeErr != nil {
			return fmt.Errorf("writing initial migration: %w", writeErr)
		}
		fmt.Printf("Created %s (from existing schema snapshot)\n", migPath)
	}

	if initialMigName != "" {
		fmt.Printf("\nYour database already has these tables applied. Mark this migration as applied without re-running SQL:\n\n  morphic migrate fake %s\n\n", initialMigName)
	} else {
		fmt.Printf("\nInitialization complete. No existing schema found.\n\nTo generate your first migration:\n  morphic generate --name \"initial\"\n\nThen run:\n  morphic migrate up\n\n")
	}

	return nil
}
