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

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/interp"
)

var convertOutputDir string

var convertCmd = &cobra.Command{
	Use:     "convert [migrations-dir]",
	Short:   "Convert Go (Yaegi) migrations to Starlark format",
	Long:    `Reads .go migration files via Yaegi, converts them to .star format, and writes the output to the specified directory.`,
	GroupID: "convert",
	Args:    cobra.ExactArgs(1),
	RunE:    runConvert,
}

func init() {
	convertCmd.Flags().StringVarP(&convertOutputDir, "output", "o", "", "Output directory for .star files (required)")
	_ = convertCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(convertCmd)
}

func runConvert(cmd *cobra.Command, args []string) error {
	migrationsDir := args[0]

	if err := os.MkdirAll(convertOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "Loading Go migrations from %s...\n", migrationsDir)

	reg, err := interp.LoadGoRegistry(migrationsDir)
	if err != nil {
		return fmt.Errorf("loading Go migrations: %w", err)
	}

	migrations := reg.All()
	if len(migrations) == 0 {
		_, _ = fmt.Fprintln(w, "No migrations found.")
		return nil
	}

	_, _ = fmt.Fprintf(w, "Found %d migration(s). Converting...\n", len(migrations))

	for _, m := range migrations {
		src, convErr := codegen.ConvertMigrationToStarlark(m)
		if convErr != nil {
			return fmt.Errorf("converting %s: %w", m.Name, convErr)
		}

		outPath := filepath.Join(convertOutputDir, m.Name+".star")
		if writeErr := os.WriteFile(outPath, []byte(src), 0644); writeErr != nil {
			return fmt.Errorf("writing %s: %w", outPath, writeErr)
		}

		_, _ = fmt.Fprintf(w, "  ✓ %s.star\n", m.Name)
	}

	_, _ = fmt.Fprintf(w, "\nConverted %d migration(s) to %s\n", len(migrations), convertOutputDir)
	return nil
}
