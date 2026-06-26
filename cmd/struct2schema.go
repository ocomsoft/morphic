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

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/struct2schema"
)

var (
	s2sInputDir    string
	s2sOutputFile  string
	s2sConfigFile  string
	s2sTargetDB    string
	s2sDryRunMode  bool
	s2sVerboseMode bool
)

// struct2schemaCmd represents the struct2schema command
var struct2schemaCmd = &cobra.Command{
	Use:     "struct-to-schema",
	Aliases: []string{"struct2schema"},
	GroupID: "convert",
	Short:   "Convert Go structs to Starlark schema format",
	Long: `Convert Go structs to Starlark schema format compatible with morphic.

This command scans Go source files in a directory, extracts struct definitions,
and generates a Starlark schema file that can be used with morphic.

Features:
- Recursive directory scanning with smart exclusions (.git, vendor, etc.)
- Struct tag parsing with priority order: db, sql, gorm, bun
- Automatic type mapping from Go types to SQL types
- Support for relationships (foreign keys, many-to-many)
- Configurable type mappings and naming conventions
- Merge with existing schema files
- Dry-run mode for preview

Examples:
  # Scan current directory and generate schema.star
  morphic struct2schema

  # Scan specific directory with custom output
  morphic struct2schema --input ./models --output schema/generated.star

  # Use custom config for type mappings
  morphic struct2schema --config mappings.yaml --database postgresql

  # Preview changes without writing files
  morphic struct2schema --dry-run --verbose`,
	RunE: runStruct2Schema,
}

func init() {
	rootCmd.AddCommand(struct2schemaCmd)

	struct2schemaCmd.Flags().StringVar(&s2sInputDir, "input", ".", "Input directory to scan for Go files")
	struct2schemaCmd.Flags().StringVar(&s2sOutputFile, "output", "schema/schema.star", "Output Starlark schema file path")
	struct2schemaCmd.Flags().StringVar(&s2sConfigFile, "config", "", "Configuration file path for custom type mappings")
	struct2schemaCmd.Flags().StringVar(&s2sTargetDB, "database", "postgresql", "Target database type (postgresql, mysql, sqlite, sqlserver)")
	struct2schemaCmd.Flags().BoolVar(&s2sDryRunMode, "dry-run", false, "Preview changes without writing files")
	struct2schemaCmd.Flags().BoolVar(&s2sVerboseMode, "verbose", false, "Show detailed processing information")
}

func runStruct2Schema(cmd *cobra.Command, args []string) error {
	if s2sVerboseMode {
		fmt.Println("struct2schema - Go struct to Starlark schema converter")
		fmt.Println("======================================================")
	}

	// Initialize the struct2schema processor
	processor, err := struct2schema.NewProcessor(struct2schema.ProcessorConfig{
		InputDir:   s2sInputDir,
		OutputFile: s2sOutputFile,
		ConfigFile: s2sConfigFile,
		TargetDB:   s2sTargetDB,
		DryRun:     s2sDryRunMode,
		Verbose:    s2sVerboseMode,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize processor: %w", err)
	}

	// Process the structs
	if err := processor.Process(); err != nil {
		return fmt.Errorf("failed to process structs: %w", err)
	}

	if s2sDryRunMode {
		fmt.Println("\nDry run completed successfully - no files were modified")
	} else {
		if s2sVerboseMode {
			fmt.Printf("\nSchema file written to: %s\n", s2sOutputFile)
		}
		fmt.Println("struct2schema completed successfully")
	}

	return nil
}
