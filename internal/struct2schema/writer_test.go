package struct2schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/types"
)

// TestNewWriter verifies writer creation.
func TestNewWriter(t *testing.T) {
	t.Parallel()

	w := NewWriter(false)
	if w == nil {
		t.Fatal("NewWriter returned nil")
	}
	if w.verbose {
		t.Error("verbose should be false")
	}
}

// TestPreviewSchema verifies that PreviewSchema produces valid Starlark output.
func TestPreviewSchema(t *testing.T) {
	t.Parallel()

	w := NewWriter(false)

	schema := &types.Schema{
		Database: types.Database{
			Name:    "test_db",
			Version: "1.0.0",
		},
		Tables: []types.Table{
			{
				Name: "users",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "name", Type: "varchar", Length: 255},
				},
			},
		},
	}

	// PreviewSchema prints to stdout; just verify no error
	err := w.PreviewSchema(schema)
	if err != nil {
		t.Fatalf("PreviewSchema: %v", err)
	}
}

// TestWriteSchemaNew verifies writing a new schema file.
func TestWriteSchemaNew(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "schema.star")

	w := NewWriter(false)

	schema := &types.Schema{
		Database: types.Database{
			Name:    "test_db",
			Version: "1.0.0",
		},
		Tables: []types.Table{
			{
				Name: "items",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "title", Type: "varchar", Length: 100},
				},
			},
		},
	}

	if err := w.WriteSchema(schema, outputPath); err != nil {
		t.Fatalf("WriteSchema: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)

	// Verify it is valid Starlark DSL
	if !strings.Contains(content, `database("test_db"`) {
		t.Errorf("expected database name in output, got:\n%s", content)
	}
	if !strings.Contains(content, `table("items"`) {
		t.Error("expected table name in output")
	}
	if !strings.Contains(content, `serial("id"`) {
		t.Error("expected serial field in output")
	}
	if !strings.Contains(content, `varchar("title", 100`) {
		t.Error("expected varchar field in output")
	}
	// Must NOT contain YAML syntax
	if strings.Contains(content, "tables:") || strings.Contains(content, "fields:") {
		t.Error("Starlark output must not contain YAML syntax")
	}
}

// TestWriteSchemaBackup verifies that writing over an existing file
// creates a backup.
func TestWriteSchemaBackup(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "schema.yaml")

	// Create an initial file
	initialContent := []byte("existing content")
	if err := os.WriteFile(outputPath, initialContent, 0644); err != nil {
		t.Fatalf("write initial: %v", err)
	}

	w := NewWriter(false)
	schema := &types.Schema{
		Database: types.Database{
			Name:    "new_db",
			Version: "2.0.0",
		},
		Tables: []types.Table{
			{
				Name: "products",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
				},
			},
		},
	}

	if err := w.WriteSchema(schema, outputPath); err != nil {
		t.Fatalf("WriteSchema: %v", err)
	}

	// Check that a backup file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	backupFound := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backup.") {
			backupFound = true
			// Verify backup content
			backupData, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("read backup: %v", err)
			}
			if string(backupData) != "existing content" {
				t.Errorf("backup content = %q, want %q", string(backupData), "existing content")
			}
			break
		}
	}
	if !backupFound {
		t.Error("expected backup file to be created")
	}
}

// TestMergeSchemas verifies schema merging logic.
func TestMergeSchemas(t *testing.T) {
	t.Parallel()

	w := NewWriter(false)

	existing := &types.Schema{
		Database: types.Database{
			Name:    "my_app",
			Version: "1.0.0",
		},
		Tables: []types.Table{
			{
				Name: "users",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "name", Type: "varchar", Length: 255},
					{Name: "legacy_field", Type: "text"},
				},
			},
			{
				Name: "old_table",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
				},
			},
		},
	}

	newSchema := &types.Schema{
		Database: types.Database{
			Name:    "generated_schema",
			Version: "1.0.0",
		},
		Tables: []types.Table{
			{
				Name: "users",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "name", Type: "varchar", Length: 100},  // changed length
					{Name: "email", Type: "varchar", Length: 255}, // new field
				},
			},
			{
				Name: "posts",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "title", Type: "varchar", Length: 200},
				},
			},
		},
	}

	merged, err := w.mergeSchemas(existing, newSchema)
	if err != nil {
		t.Fatalf("mergeSchemas: %v", err)
	}

	// Database name should come from existing since new is "generated_schema"
	if merged.Database.Name != "my_app" {
		t.Errorf("database name = %q, want %q", merged.Database.Name, "my_app")
	}

	// Should have 3 tables: users (merged), posts (new), old_table (preserved)
	if len(merged.Tables) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(merged.Tables))
	}

	tableNames := map[string]bool{}
	for _, table := range merged.Tables {
		tableNames[table.Name] = true
	}
	if !tableNames["users"] {
		t.Error("users table missing")
	}
	if !tableNames["posts"] {
		t.Error("posts table missing")
	}
	if !tableNames["old_table"] {
		t.Error("old_table should be preserved")
	}

	// Check users table has merged fields
	for _, table := range merged.Tables {
		if table.Name == "users" {
			fieldNames := map[string]bool{}
			for _, f := range table.Fields {
				fieldNames[f.Name] = true
			}
			if !fieldNames["email"] {
				t.Error("email field should be added from new schema")
			}
			if !fieldNames["legacy_field"] {
				t.Error("legacy_field should be preserved from existing")
			}
			break
		}
	}
}

// TestMergeFields verifies field merging preserves manual defaults.
func TestMergeFields(t *testing.T) {
	t.Parallel()

	w := NewWriter(false)

	existing := &types.Field{
		Name:    "status",
		Type:    "varchar",
		Length:  50,
		Default: "active",
	}

	newField := &types.Field{
		Name:   "status",
		Type:   "varchar",
		Length: 100, // updated
		// No default
	}

	merged := w.mergeFields(existing, newField)

	if merged.Length != 100 {
		t.Errorf("Length = %d, want 100", merged.Length)
	}
	if merged.Default != "active" {
		t.Errorf("Default = %q, want %q (preserved from existing)", merged.Default, "active")
	}
}

// TestCreateOutputDirectory verifies directory creation.
func TestCreateOutputDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "subdir", "nested", "schema.yaml")

	w := NewWriter(false)
	if err := w.CreateOutputDirectory(outputPath); err != nil {
		t.Fatalf("CreateOutputDirectory: %v", err)
	}

	// Verify the directory was created
	parentDir := filepath.Dir(outputPath)
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}

// TestValidateOutputPath verifies output path validation.
func TestValidateOutputPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "schema.yaml")

	w := NewWriter(false)
	if err := w.ValidateOutputPath(outputPath); err != nil {
		t.Fatalf("ValidateOutputPath: %v", err)
	}
}

// TestValidateOutputPathCreatesDir verifies that validation creates
// missing directories.
func TestValidateOutputPathCreatesDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "new_subdir", "schema.yaml")

	w := NewWriter(false)
	if err := w.ValidateOutputPath(outputPath); err != nil {
		t.Fatalf("ValidateOutputPath: %v", err)
	}

	parentDir := filepath.Dir(outputPath)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("expected directory to be created by validation")
	}
}
