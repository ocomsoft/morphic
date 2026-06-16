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
	"strings"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/config"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

// ExecuteGoMigrationInit initializes the migrations/ directory for the Go migration framework.
// If Goose-style *.sql files are detected in the migrations directory, it automatically
// delegates to ExecuteMigrateToGo to convert them. Otherwise, if a .schema_snapshot.yaml
// exists it generates an initial Go migration from it. Always generates main.go and go.mod
// if they don't already exist.
func ExecuteGoMigrationInit(databaseType string, verbose bool) error {
	cfg := config.DefaultConfig()
	migrationsDir := cfg.Migration.Directory
	gen := newGenerator(cfg)
	format := codegen.ParseMigrationFormat(cfg.Migration.Format)

	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		return fmt.Errorf("creating migrations directory: %w", err)
	}

	//// Auto-upgrade: if Goose SQL migrations exist, convert them to Go migrations.
	//sqlFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	//if len(sqlFiles) > 0 {
	//	fmt.Printf("Detected %d Goose SQL migration(s) in %s — running migrate-to-go...\n",
	//		len(sqlFiles), migrationsDir)
	//	return ExecuteMigrateToGo(migrationsDir, false, true, false, os.Stdout)
	//}

	var initialMigName string

	// Detect existing snapshot and generate initial migration from it
	sm := yamlpkg.NewStateManager(verbose)
	existingSchema, err := sm.LoadSchemaSnapshot(migrationsDir)
	if err == nil && existingSchema != nil {
		initialMigName = "0001_initial"
		diff := schemaToInitialDiff(existingSchema, databaseType)
		src, err := gen.GenerateMigration(initialMigName, []string{}, diff, existingSchema, nil, nil)
		if err != nil {
			return fmt.Errorf("generating initial migration: %w", err)
		}
		migPath := filepath.Join(migrationsDir, initialMigName+gen.FileExtension())
		if err := os.WriteFile(migPath, []byte(src), 0644); err != nil {
			return fmt.Errorf("writing initial migration: %w", err)
		}
		fmt.Printf("Created %s (from existing schema snapshot)\n", migPath)
	}

	// Generate main.go and go.mod only for Go format (Starlark doesn't need them).
	if format == codegen.FormatGo {
		goGen, ok := gen.(*codegen.GoGenerator)
		if ok {
			mainPath := filepath.Join(migrationsDir, "main.go")
			if _, statErr := os.Stat(mainPath); os.IsNotExist(statErr) {
				if err := os.WriteFile(mainPath, []byte(goGen.GenerateMainGo()), 0644); err != nil {
					return fmt.Errorf("writing main.go: %w", err)
				}
				fmt.Printf("Created %s\n", mainPath)
			}

			moduleName := readModuleName() + "/migrations"
			parentGoVersion := findParentGoVersion(".")

			goModPath := filepath.Join(migrationsDir, "go.mod")
			if _, statErr := os.Stat(goModPath); os.IsNotExist(statErr) {
				if err := os.WriteFile(goModPath, []byte(goGen.GenerateGoMod(moduleName, "main", parentGoVersion)), 0644); err != nil {
					return fmt.Errorf("writing go.mod: %w", err)
				}
				fmt.Printf("Created %s\n", goModPath)
			}
		}
	}

	// Print next steps
	if initialMigName != "" {
		fmt.Printf(`
Your database already has these tables applied. Mark this migration as applied without re-running SQL:

  morphic migrate fake %s

`, initialMigName)
	} else {
		fmt.Printf(`
Initialization complete. No existing schema found.

To generate your first migration:
  morphic generate --name "initial"

Then run:
  morphic migrate up

Migrations are interpreted in-process — no Go toolchain required at runtime.
The generated %s/main.go and %s/go.mod remain available so you can still
'go build' the migrations module for IDE type-checking or as a fallback.
`, migrationsDir, migrationsDir)
	}

	return nil
}

// schemaToInitialDiff converts a yaml.Schema to a SchemaDiff that treats every
// table as newly added. Used to generate the initial Go migration from an
// existing .schema_snapshot.yaml.
// If the schema has non-empty defaults for dbType, a SetDefaults change is
// prepended so the initial migration tracks the defaults in the DAG.
func schemaToInitialDiff(schema *yamlpkg.Schema, dbType string) *yamlpkg.SchemaDiff {
	diff := &yamlpkg.SchemaDiff{HasChanges: true}

	// Prepend SetDefaults if the schema has defaults for this DB type
	if defaults := getDefaultsForDB(schema, dbType); len(defaults) > 0 {
		diff.Changes = append(diff.Changes, yamlpkg.Change{
			Type:        yamlpkg.ChangeTypeDefaultsModified,
			Description: "Set initial schema defaults",
			NewValue:    defaults,
		})
	}

	// Prepend SetTypeMappings if the schema has type mappings for this DB type
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

// readModuleName reads the Go module name from go.mod in the current working directory.
// Returns "myproject" if go.mod is not found or cannot be parsed.
func readModuleName() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "myproject"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return "myproject"
}
