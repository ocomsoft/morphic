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

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/yaml"
)

var yaml2dslCmd = &cobra.Command{
	Use:     "yaml2dsl <input.yaml> <output.star>",
	Short:   "Convert a YAML schema file to Starlark DSL format",
	Long:    `Reads a YAML schema file, converts it to the Starlark schema DSL, and writes the .star output file.`,
	GroupID: "convert",
	Args:    cobra.ExactArgs(2),
	RunE:    runYAML2DSL,
}

func init() {
	rootCmd.AddCommand(yaml2dslCmd)
}

func runYAML2DSL(cmd *cobra.Command, args []string) error {
	inputPath := args[0]
	outputPath := args[1]

	w := cmd.OutOrStdout()

	_, _ = fmt.Fprintf(w, "Reading YAML schema from %s...\n", inputPath)

	parser := yaml.NewParser(verbose)
	schema, err := parser.ParseSchemaFile(inputPath)
	if err != nil {
		return fmt.Errorf("parsing YAML schema: %w", err)
	}

	dsl, err := codegen.GenerateSchemaDSL(schema)
	if err != nil {
		return fmt.Errorf("generating Starlark DSL: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(dsl), 0644); err != nil {
		return fmt.Errorf("writing output file: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Wrote Starlark schema to %s\n", outputPath)
	return nil
}
