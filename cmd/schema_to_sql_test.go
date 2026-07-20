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
package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/internal/workflow"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
	"github.com/spf13/cobra"
)

// setupSchemaDir creates a temp directory with a go.mod and a Starlark schema
// file containing the given schema content, then chdir into it. It returns a
// cleanup function that restores the original working directory.
func setupSchemaDir(t *testing.T, schemaContent string) func() {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	// Create go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testproject\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}

	// Create schema directory and schema.star
	schemaDir := filepath.Join(dir, "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatalf("MkdirAll schema: %v", err)
	}
	if err := os.WriteFile(filepath.Join(schemaDir, "schema.star"), []byte(schemaContent), 0644); err != nil {
		t.Fatalf("WriteFile schema.star: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restoring working directory: %v", err)
		}
	}
}

// newTestCmd creates a cobra.Command with captured stderr for testing.
func newTestCmd() (*cobra.Command, *bytes.Buffer) {
	var buf bytes.Buffer
	c := &cobra.Command{}
	c.SetErr(&buf)
	return c, &buf
}

// TestExecuteDumpSQL_FullSchema verifies that full schema dump mode (pending=false)
// generates SQL output for a simple schema.
func TestExecuteDumpSQL_FullSchema(t *testing.T) {
	schema := `database("users_db", "1.0.0")

table("users",
    fields = [
        serial("id", primary_key=True),
        text("email"),
    ],
)
`
	cleanup := setupSchemaDir(t, schema)
	defer cleanup()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	testCmd, _ := newTestCmd()
	err := workflow.ExecuteDumpSQL(testCmd, "postgresql", false, false, "", nil)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("ExecuteDumpSQL: %v", err)
	}

	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	output := out.String()

	if len(output) == 0 {
		t.Fatal("expected SQL output, got empty string")
	}

	// Should contain CREATE TABLE for the users table
	if !bytes.Contains([]byte(output), []byte("CREATE TABLE")) {
		t.Errorf("expected CREATE TABLE in output, got:\n%s", output)
	}
	if !bytes.Contains([]byte(output), []byte("users")) {
		t.Errorf("expected 'users' table in output, got:\n%s", output)
	}
}

// TestExecuteDumpSQL_NoPendingChanges verifies that pending mode with no migrations
// directory still works (treats all changes as pending since there's no previous state).
func TestExecuteDumpSQL_NoPendingChanges(t *testing.T) {
	schema := `database("users_db", "1.0.0")

table("users",
    fields = [
        serial("id", primary_key=True),
        text("email"),
    ],
)
`
	cleanup := setupSchemaDir(t, schema)
	defer cleanup()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	testCmd, _ := newTestCmd()
	// With pending=true and no migrations directory, all changes are "pending"
	// since previous state is empty
	dagQuerier := &workflow.DAGQuerier{
		QueryPreviousSchema: func(migrationsDir string, dbType string, verbose bool) (*yamlpkg.Schema, error) {
			return nil, nil
		},
	}
	err := workflow.ExecuteDumpSQL(testCmd, "postgresql", true, false, "", dagQuerier)

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("ExecuteDumpSQL pending: %v", err)
	}

	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	output := out.String()

	// Should produce SQL since everything is new (no previous state)
	if len(output) == 0 {
		t.Fatal("expected pending SQL output for new schema, got empty string")
	}

	// Should contain CREATE TABLE since all tables are new
	if !bytes.Contains([]byte(output), []byte("CREATE TABLE")) {
		t.Errorf("expected CREATE TABLE in pending output, got:\n%s", output)
	}
}

// TestExecuteDumpSQL_NoSchemaFiles verifies graceful handling when no schema
// files are found.
func TestExecuteDumpSQL_NoSchemaFiles(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restoring working directory: %v", err)
		}
	}()

	// Create go.mod but no schema files
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testproject\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatalf("WriteFile go.mod: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	testCmd, _ := newTestCmd()
	err = workflow.ExecuteDumpSQL(testCmd, "postgresql", false, false, "", nil)
	if err != nil {
		t.Fatalf("expected nil error for no schema files, got: %v", err)
	}
}
