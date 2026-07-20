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
package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/errors"
)

func TestScanner_ScanModules_ValidModule(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "scanner_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create go.mod
	goModContent := `module test/module

go 1.21

require (
	github.com/example/dep v1.0.0
)
`
	err = os.WriteFile("go.mod", []byte(goModContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create schema.sql
	sqlDir := filepath.Join("sql")
	if err = os.MkdirAll(sqlDir, 0755); err != nil {
		t.Fatal(err)
	}
	schemaContent := `-- MIGRATION_SCHEMA
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL
);
`
	err = os.WriteFile(filepath.Join(sqlDir, "schema.sql"), []byte(schemaContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	scanner := New(false)
	schemas, err := scanner.ScanModules()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(schemas) != 1 {
		t.Fatalf("Expected 1 schema, got %d", len(schemas))
	}

	schema := schemas[0]
	if schema.ModulePath != "current module" {
		t.Errorf("Expected module path 'current module', got '%s'", schema.ModulePath)
	}

	if !schema.HasMarker {
		t.Error("Expected schema to have MIGRATION_SCHEMA marker")
	}

	if !strings.Contains(schema.Content, "CREATE TABLE users") {
		t.Error("Expected schema content to contain CREATE TABLE users")
	}
}

func TestScanner_ScanModules_NoGoMod(t *testing.T) {
	// Create temporary directory without go.mod
	tmpDir, err := os.MkdirTemp("", "scanner_test_no_mod")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	scanner := New(false)
	_, err = scanner.ScanModules()

	if err == nil {
		t.Fatal("Expected error for missing go.mod")
	}

	if !errors.IsValidationError(err) {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestScanner_ScanModules_EmptyGoMod(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "scanner_test_empty")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create empty go.mod
	err = os.WriteFile("go.mod", []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	scanner := New(false)
	_, err = scanner.ScanModules()

	if err == nil {
		t.Fatal("Expected error for empty go.mod")
	}

	if !errors.IsValidationError(err) {
		t.Errorf("Expected ValidationError, got %T", err)
	}
}

func TestScanner_ScanStarlarkModules_FindsStarFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scanner_star_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	goModContent := "module test/module\n\ngo 1.21\n"
	if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	schemaDir := filepath.Join("modules", "tenant", "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatal(err)
	}
	starContent := `database(name="tenant", version="1.0")`
	if err := os.WriteFile(filepath.Join(schemaDir, "schema.star"), []byte(starContent), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(false)
	schemas, err := s.ScanStarlarkModules()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(schemas) != 1 {
		t.Fatalf("Expected 1 schema, got %d", len(schemas))
	}

	schema := schemas[0]
	if schema.ModulePath != "current module" {
		t.Errorf("Expected module path 'current module', got '%s'", schema.ModulePath)
	}
	if schema.Type != SchemaTypeStarlark {
		t.Errorf("Expected type SchemaTypeStarlark, got '%s'", schema.Type)
	}
	if !schema.HasMarker {
		t.Error("Expected HasMarker to be true for Starlark schemas")
	}
	if !strings.Contains(schema.Content, "database(") {
		t.Error("Expected schema content to contain database( call")
	}
}

func TestScanner_ScanStarlarkModules_MultipleModules(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scanner_star_multi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	goModContent := "module test/module\n\ngo 1.21\n"
	if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	modules := []string{"tenant", "code_data", "files_data"}
	for _, mod := range modules {
		schemaDir := filepath.Join("modules", mod, "schema")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`database(name="%s", version="1.0")`, mod)
		if err := os.WriteFile(filepath.Join(schemaDir, "schema.star"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s := New(false)
	schemas, err := s.ScanStarlarkModules()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(schemas) != 3 {
		t.Fatalf("Expected 3 schemas, got %d", len(schemas))
	}
}

func TestScanner_ScanStarlarkModules_IgnoresYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scanner_star_no_yaml_test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err = os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	goModContent := "module test/module\n\ngo 1.21\n"
	if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a schema.yaml file — this should NOT be found
	schemaDir := filepath.Join("modules", "tenant", "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(schemaDir, "schema.yaml"), []byte("database:\n  name: tenant\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(false)
	schemas, err := s.ScanStarlarkModules()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(schemas) != 0 {
		t.Fatalf("Expected 0 schemas (YAML should be ignored), got %d", len(schemas))
	}
}

func TestScanner_readSchemaFile(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedMarker bool
	}{
		{
			name: "with marker",
			content: `-- MIGRATION_SCHEMA
CREATE TABLE test (id INT);`,
			expectedMarker: true,
		},
		{
			name:           "without marker",
			content:        `CREATE TABLE test (id INT);`,
			expectedMarker: false,
		},
		{
			name: "marker with whitespace",
			content: `   -- MIGRATION_SCHEMA   
CREATE TABLE test (id INT);`,
			expectedMarker: true,
		},
	}

	scanner := New(false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.content)
			content, hasMarker, err := scanner.readSchemaFile(reader)

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if hasMarker != tt.expectedMarker {
				t.Errorf("Expected marker %v, got %v", tt.expectedMarker, hasMarker)
			}

			if content != tt.content {
				t.Errorf("Expected content %q, got %q", tt.content, content)
			}
		})
	}
}
