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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
	yaml "gopkg.in/yaml.v3"

	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/workflow"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

var (
	interactive      bool
	schemaPath       string
	includeWorkspace bool
)

// findIncludesCmd represents the find_includes command
var findIncludesCmd = &cobra.Command{
	Use:     "find-includes",
	Aliases: []string{"find_includes"},
	GroupID: "convert",
	Short:   "Discover and add schema includes from Go modules",
	Long: `Automatically discover Starlark schema files in Go modules and workspace,
then add them as includes to your main schema file.

This command searches for schema.star files in any subdirectory within:
1. Go workspace modules (prioritized and marked as "recommended")
2. Direct dependencies in go.mod

By default, all discovered schemas are added to the include section.
Use --interactive to review and select which schemas to include.

The command preserves existing includes and only adds newly discovered ones.

Examples:
  morphic find_includes                    # Add all discovered schemas
  morphic find_includes --interactive      # Review before adding
  morphic find_includes --schema custom.star  # Use different schema file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runFindIncludes(cmd, args)
	},
}

// runFindIncludes executes the find_includes command
func runFindIncludes(cmd *cobra.Command, args []string) error {
	callbacks := &workflow.FindIncludesCallbacks{
		FindLocalSchemaFiles: func() ([]workflow.LocalSchemaFile, error) {
			return findLocalSchemaFiles()
		},
		SelectLocalSchemaFile: func(schemas []workflow.LocalSchemaFile) (string, error) {
			return selectLocalSchemaFile(schemas)
		},
		DiscoverSchemas: func() ([]workflow.DiscoveredSchema, error) {
			return discoverSchemas()
		},
		SelectSchemasInteractively: func(schemas []workflow.DiscoveredSchema) ([]workflow.DiscoveredSchema, error) {
			return selectSchemasInteractively(schemas)
		},
		LoadExistingSchema: func(path string) (*yamlpkg.Schema, error) {
			schemaDir := filepath.Dir(path)
			return interp.LoadSchema(schemaDir, false)
		},
		FilterNewSchemas: func(discovered []workflow.DiscoveredSchema, existing *yamlpkg.Schema) []workflow.DiscoveredSchema {
			return filterNewSchemas(discovered, existing)
		},
		UpdateSchemaWithIncludes: func(path string, schema *yamlpkg.Schema, schemasToAdd []workflow.DiscoveredSchema) error {
			return updateSchemaWithIncludes(path, schema, schemasToAdd)
		},
	}
	return workflow.ExecuteFindIncludes(cmd, cfgFile, schemaPath, interactive, includeWorkspace, callbacks)
}

// discoverSchemas finds all schemas in Go modules and workspace
func discoverSchemas() ([]workflow.DiscoveredSchema, error) {
	var discovered []workflow.DiscoveredSchema

	// First, discover workspace modules (prioritized)
	if includeWorkspace {
		workspaceSchemas, err := discoverWorkspaceSchemas()
		if err != nil {
			if verbose {
				color.Yellow("Warning: Failed to discover workspace schemas: %v", err)
			}
		} else {
			discovered = append(discovered, workspaceSchemas...)
		}
	}

	// Then, discover go.mod dependencies
	modSchemas, err := discoverGoModSchemas()
	if err != nil {
		return nil, fmt.Errorf("failed to discover go.mod schemas: %w", err)
	}
	discovered = append(discovered, modSchemas...)

	return discovered, nil
}

// discoverWorkspaceSchemas finds schemas in Go workspace modules
// discoverWorkspaceSchemas finds schemas in Go workspace modules
func discoverWorkspaceSchemas() ([]workflow.DiscoveredSchema, error) {
	var discovered []workflow.DiscoveredSchema

	// Find go.work file by searching upward through parent directories
	workFilePath, err := findGoWorkFile()
	if err != nil {
		if verbose {
			color.Yellow("No go.work file found, skipping workspace modules")
		}
		return discovered, nil
	}

	if verbose {
		color.Blue("Found go.work file at: %s", workFilePath)
	}

	// Parse go.work
	workBytes, err := os.ReadFile(workFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.work: %w", err)
	}

	workFile, err := modfile.ParseWork(workFilePath, workBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.work: %w", err)
	}

	// Get the directory containing go.work to resolve relative paths
	workDir := filepath.Dir(workFilePath)

	// Compute absolute path of the target schema (the one we're updating) for exclusion
	var targetAbs string
	if schemaPath != "" {
		if abs, err := filepath.Abs(schemaPath); err == nil {
			targetAbs = abs
		}
	}

	// Scan each workspace directory
	for _, use := range workFile.Use {
		// Resolve relative paths against the go.work directory
		usePath := use.Path
		if !filepath.IsAbs(usePath) {
			usePath = filepath.Join(workDir, use.Path)
		}

		if verbose {
			color.Cyan("Scanning workspace module: %s", usePath)
		}

		schemas, err := findSchemasInPath(usePath, true)
		if err != nil {
			if verbose {
				color.Yellow("  Warning: %v", err)
			}
			continue
		}

		// Exclude the schema file we're trying to add to
		if targetAbs != "" {
			filtered := make([]workflow.DiscoveredSchema, 0, len(schemas))
			for _, s := range schemas {
				if s.FullPath == targetAbs {
					if verbose {
						color.Yellow("    Skipping current schema file: %s", s.FullPath)
					}
					continue
				}
				filtered = append(filtered, s)
			}
			schemas = filtered
		}

		discovered = append(discovered, schemas...)
	}

	return discovered, nil
}

// ... existing code ...

// findGoWorkFile searches for go.work file starting from current directory and moving up
func findGoWorkFile() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	dir := currentDir
	for {
		workFile := filepath.Join(dir, "go.work")
		if _, err := os.Stat(workFile); err == nil {
			return workFile, nil
		}

		parentDir := filepath.Dir(dir)
		// Stop if we've reached the root directory
		if parentDir == dir {
			break
		}
		dir = parentDir
	}

	return "", fmt.Errorf("go.work file not found in current directory or any parent directory")
}

// discoverGoModSchemas finds schemas in go.mod dependencies
func discoverGoModSchemas() ([]workflow.DiscoveredSchema, error) {
	var discovered []workflow.DiscoveredSchema

	// Parse go.mod
	goModBytes, err := os.ReadFile("go.mod")
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod: %w", err)
	}

	modFile, err := modfile.Parse("go.mod", goModBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	// Scan direct dependencies
	for _, req := range modFile.Require {
		if req.Indirect {
			continue // Skip indirect dependencies
		}

		if verbose {
			color.Cyan("Scanning module: %s@%s", req.Mod.Path, req.Mod.Version)
		}

		modPath := getModuleCachePath(req.Mod.Path, req.Mod.Version)
		if modPath == "" {
			if verbose {
				color.Yellow("  Module not found in cache")
			}
			continue
		}

		schemas, err := findSchemasInModule(modPath, req.Mod.Path)
		if err != nil {
			if verbose {
				color.Yellow("  Warning: %v", err)
			}
			continue
		}

		discovered = append(discovered, schemas...)
	}

	return discovered, nil
}

// findSchemasInPath finds schemas in a given path (workspace module)
func findSchemasInPath(basePath string, isWorkspace bool) ([]workflow.DiscoveredSchema, error) {
	var schemas []workflow.DiscoveredSchema

	// Read the module path from go.mod in this directory
	goModPath := filepath.Join(basePath, "go.mod")
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go.mod in %s: %w", basePath, err)
	}

	modFile, err := modfile.Parse(goModPath, goModBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod in %s: %w", basePath, err)
	}

	if modFile.Module == nil {
		return nil, fmt.Errorf("no module declaration in %s", goModPath)
	}

	modulePath := modFile.Module.Mod.Path

	// Walk the directory to find schema files
	err = filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Skip directories starting with _ or .
		if d.IsDir() && (strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}

		// Look for schema.star files in any subdirectory
		if d.Name() == "schema.star" {
			relPath, err := filepath.Rel(basePath, path)
			if err != nil {
				return nil // Continue walking
			}

			// Load the schema using interp.LoadSchema on the parent directory
			schemaDir := filepath.Dir(path)
			schema, err := interp.LoadSchema(schemaDir, false)
			if err != nil {
				if verbose {
					color.Yellow("    Warning: Failed to load %s: %v", path, err)
				}
				return nil // Continue walking
			}

			discovered := workflow.DiscoveredSchema{
				ModulePath:   modulePath,
				RelativePath: relPath,
				FullPath:     path,
				IsWorkspace:  isWorkspace,
				Schema:       schema,
				TableCount:   len(schema.Tables),
				DatabaseName: schema.Database.Name,
				DatabaseType: "starlark",
			}

			schemas = append(schemas, discovered)

			if verbose {
				color.Green("    Found schema: %s (%d tables)", relPath, len(schema.Tables))
			}
		}

		return nil
	})

	return schemas, err
}

// findSchemasInModule finds schemas in a cached Go module
func findSchemasInModule(modPath, modulePath string) ([]workflow.DiscoveredSchema, error) {
	var schemas []workflow.DiscoveredSchema

	// Walk the module directory to find schema files
	err := filepath.WalkDir(modPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Skip directories starting with _ or .
		if d.IsDir() && (strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}

		// Look for schema.star files in any subdirectory
		if d.Name() == "schema.star" {
			relPath, err := filepath.Rel(modPath, path)
			if err != nil {
				return nil // Continue walking
			}

			schemaDir := filepath.Dir(path)
			schema, err := interp.LoadSchema(schemaDir, false)
			if err != nil {
				if verbose {
					color.Yellow("    Warning: Failed to load %s: %v", path, err)
				}
				return nil // Continue walking
			}

			discovered := workflow.DiscoveredSchema{
				ModulePath:   modulePath,
				RelativePath: relPath,
				FullPath:     path,
				IsWorkspace:  false,
				Schema:       schema,
				TableCount:   len(schema.Tables),
				DatabaseName: schema.Database.Name,
				DatabaseType: "starlark",
			}

			schemas = append(schemas, discovered)

			if verbose {
				color.Green("    Found schema: %s (%d tables)", relPath, len(schema.Tables))
			}
		}

		return nil
	})

	return schemas, err
}

// findLocalSchemaFiles recursively searches for schema.star files in the current directory
func findLocalSchemaFiles() ([]workflow.LocalSchemaFile, error) {
	var schemas []workflow.LocalSchemaFile
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	err = filepath.WalkDir(currentDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Skip directories starting with _ or .
		if d.IsDir() && (strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}

		// Look for schema.star files
		if d.Name() == "schema.star" {
			schemaDir := filepath.Dir(path)
			schema, err := interp.LoadSchema(schemaDir, false)
			if err != nil {
				if verbose {
					color.Yellow("Warning: Failed to load %s: %v", path, err)
				}
				return nil // Continue walking
			}

			relPath, err := filepath.Rel(currentDir, path)
			if err != nil {
				relPath = path // Use absolute path if relative fails
			}

			localSchema := workflow.LocalSchemaFile{
				Path:         relPath,
				DatabaseName: schema.Database.Name,
				TableCount:   len(schema.Tables),
			}

			schemas = append(schemas, localSchema)

			if verbose {
				color.Green("Found local schema: %s (database: %s, %d tables)", relPath, schema.Database.Name, len(schema.Tables))
			}
		}

		return nil
	})

	return schemas, err
}

// getModuleCachePath gets the path to a module in the Go module cache
func getModuleCachePath(modPath, version string) string {
	// Try to find the module in the Go module cache
	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		goPath = filepath.Join(home, "go")
	}

	// Clean the version string
	version = strings.TrimSuffix(version, "+incompatible")

	// Try standard module cache location
	cachePath := filepath.Join(goPath, "pkg", "mod", fmt.Sprintf("%s@%s", modPath, version))
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath
	}

	// Try with escaped module path
	escapedPath := strings.ReplaceAll(modPath, "/", "!")
	cachePath = filepath.Join(goPath, "pkg", "mod", fmt.Sprintf("%s@%s", escapedPath, version))
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath
	}

	return ""
}

// filterNewSchemas filters out schemas that are already included
func filterNewSchemas(discovered []workflow.DiscoveredSchema, existingSchema *yamlpkg.Schema) []workflow.DiscoveredSchema {
	var newSchemas []workflow.DiscoveredSchema

	// Create a map of existing includes for quick lookup
	existingIncludes := make(map[string]bool)
	for _, include := range existingSchema.Include {
		key := include.Module + "|" + include.Path
		existingIncludes[key] = true
	}

	// Filter out already included schemas
	for _, schema := range discovered {
		key := schema.ModulePath + "|" + schema.RelativePath
		if !existingIncludes[key] {
			newSchemas = append(newSchemas, schema)
		} else if verbose {
			color.Yellow("  Skipping already included: %s -> %s", schema.ModulePath, schema.RelativePath)
		}
	}

	return newSchemas
}

// selectLocalSchemaFile prompts user to select a schema file when multiple are found
func selectLocalSchemaFile(schemas []workflow.LocalSchemaFile) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	color.Cyan("\nMultiple schema files found:")
	color.Cyan("=" + strings.Repeat("=", 35))

	for i, schema := range schemas {
		fmt.Printf("\n%d. Path: %s\n", i+1, schema.Path)
		fmt.Printf("   Database: %s\n", schema.DatabaseName)
		fmt.Printf("   Tables: %d\n", schema.TableCount)
	}

	fmt.Printf("\nWhich schema file would you like to update? [1-%d]: ", len(schemas))

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	input = strings.TrimSpace(input)

	// Parse selection
	var selection int
	if _, err := fmt.Sscanf(input, "%d", &selection); err != nil {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	if selection < 1 || selection > len(schemas) {
		return "", fmt.Errorf("selection out of range: %d", selection)
	}

	selectedSchema := schemas[selection-1]
	color.Green("Selected: %s (database: %s)", selectedSchema.Path, selectedSchema.DatabaseName)

	return selectedSchema.Path, nil
}

// selectSchemasInteractively allows user to select which schemas to include
func selectSchemasInteractively(schemas []workflow.DiscoveredSchema) ([]workflow.DiscoveredSchema, error) {
	var selected []workflow.DiscoveredSchema
	reader := bufio.NewReader(os.Stdin)

	color.Cyan("\nDiscovered schemas (workspace modules are recommended):")
	color.Cyan("=" + strings.Repeat("=", 50))

	for i, schema := range schemas {
		fmt.Printf("\n%d. Module: %s\n", i+1, schema.ModulePath)
		fmt.Printf("   Path: %s\n", schema.RelativePath)
		fmt.Printf("   Database: %s\n", schema.DatabaseName)
		fmt.Printf("   Tables: %d\n", schema.TableCount)

		if schema.IsWorkspace {
			color.Green("   Type: Workspace module (recommended)")
		} else {
			fmt.Printf("   Type: Dependency")
		}

		fmt.Printf("\n   Include this schema? [Y/n]: ")

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		input = strings.TrimSpace(strings.ToLower(input))
		if input == "" || input == "y" || input == "yes" {
			selected = append(selected, schema)
			color.Green("   ✓ Selected\n")
		} else {
			color.Yellow("   ✗ Skipped\n")
		}
	}

	return selected, nil
}

// updateSchemaWithIncludes adds the selected includes to the schema file
func updateSchemaWithIncludes(filePath string, schema *yamlpkg.Schema, schemasToAdd []workflow.DiscoveredSchema) error {
	// Add new includes
	for _, discoveredSchema := range schemasToAdd {
		include := yamlpkg.Include{
			Module: discoveredSchema.ModulePath,
			Path:   discoveredSchema.RelativePath,
		}
		schema.Include = append(schema.Include, include)
	}

	// Marshal back to YAML
	content, err := yaml.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Write back to file
	err = os.WriteFile(filePath, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(findIncludesCmd)

	// Add flags
	findIncludesCmd.Flags().BoolVar(&interactive, "interactive", false,
		"Review and select which schemas to include")
	findIncludesCmd.Flags().StringVar(&schemaPath, "schema", "",
		"Path to the main schema file to update (if not provided, will search for schema.star files)")
	findIncludesCmd.Flags().BoolVar(&includeWorkspace, "workspace", true,
		"Include workspace modules in discovery")
	findIncludesCmd.Flags().BoolVar(&verbose, "verbose", false,
		"Show detailed processing information")

	// Track if schema flag was provided
	findIncludesCmd.Flags().Lookup("schema").DefValue = ""
}
