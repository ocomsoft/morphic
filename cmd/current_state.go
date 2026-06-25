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
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/ocomsoft/morphic/internal/config"
)

// currentStateVerbose controls verbose output for the current_state command.
var currentStateVerbose bool

// currentStateCmd reconstructs the schema state from existing Go migration
// files and outputs it as YAML. This is useful for debugging migration drift
// — you can compare the output against your schema.yaml to see exactly what
// the migration DAG thinks the current schema looks like.
var currentStateCmd = &cobra.Command{
	Use:     "current-state",
	Aliases: []string{"current_state"},
	GroupID: "inspect",
	Short:   "Show the reconstructed schema state from existing migrations",
	Long: `Rebuilds the merged schema from all existing Go migration files by walking the
migration DAG and applying each operation's Mutate in order. The resulting
schema state is output as YAML.

This is useful for:
  - Debugging why morphic generate keeps generating the same migration
  - Verifying that the migration chain produces the expected schema
  - Comparing the reconstructed state against your schema.yaml files`,
	RunE: runCurrentState,
}

func init() {
	rootCmd.AddCommand(currentStateCmd)
	currentStateCmd.Flags().BoolVar(&currentStateVerbose, "verbose", false,
		"Show detailed output")
}

// runCurrentState queries the migration DAG, reconstructs the schema, and
// prints it as YAML to stdout.
func runCurrentState(_ *cobra.Command, _ []string) error {
	cfg := config.LoadOrDefault(cfgFile)
	migrationsDir := cfg.Migration.Directory

	goFiles, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	if err != nil {
		return fmt.Errorf("scanning migrations directory: %w", err)
	}

	var migFiles []string
	for _, f := range goFiles {
		if filepath.Base(f) != "main.go" {
			migFiles = append(migFiles, f)
		}
	}

	if len(migFiles) == 0 {
		fmt.Println("No migration files found.")
		return nil
	}

	dagOut, err := queryDAG(migrationsDir, currentStateVerbose)
	if err != nil {
		return fmt.Errorf("querying migration DAG: %w", err)
	}

	schema := schemaStateToYAMLSchema(dagOut.SchemaState, cfg.Database.Type)
	if schema == nil {
		fmt.Println("No schema state reconstructed.")
		return nil
	}

	out, err := yaml.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshalling schema: %w", err)
	}

	fmt.Print(string(out))
	return nil
}
