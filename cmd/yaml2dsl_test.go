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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYAML2DSLCommand(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "schema.yaml")
	outputPath := filepath.Join(dir, "schema.star")

	yamlContent := `
database:
  name: testapp
  version: "1.0.0"
defaults:
  postgresql:
    blank: "''"
    now: CURRENT_TIMESTAMP
tables:
  - name: users
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: email
        type: varchar
        length: 255
      - name: active
        type: boolean
    indexes:
      - name: idx_users_email
        fields: [email]
        unique: true
`
	if err := os.WriteFile(inputPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute the command.
	rootCmd.SetArgs([]string{"yaml2dsl", inputPath, outputPath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("yaml2dsl command failed: %v", err)
	}

	// Read and verify output.
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	output := string(data)

	checks := []string{
		`database("testapp", "1.0.0")`,
		`defaults("postgresql"`,
		`table("users"`,
		`serial("id"`,
		`varchar("email", 255`,
		`boolean("active"`,
		`index("idx_users_email"`,
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q\n\nFull output:\n%s", check, output)
		}
	}
}

func TestYAML2DSLCommand_MissingInput(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "schema.star")

	rootCmd.SetArgs([]string{"yaml2dsl", filepath.Join(dir, "nonexistent.yaml"), outputPath})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
}
