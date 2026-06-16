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

package migrate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/providers/postgresql"
	"github.com/ocomsoft/morphic/internal/providers/sqlite"
	"github.com/ocomsoft/morphic/migrate"
)

// TestUpsertData_TypeName verifies the operation identifier.
func TestUpsertData_TypeName(t *testing.T) {
	op := &migrate.UpsertData{Table: "t", ConflictKeys: []string{"id"}}
	if op.TypeName() != "upsert_data" {
		t.Errorf("TypeName: got %q", op.TypeName())
	}
}

// TestUpsertData_TableName verifies the table name accessor.
func TestUpsertData_TableName(t *testing.T) {
	op := &migrate.UpsertData{Table: "countries"}
	if op.TableName() != "countries" {
		t.Errorf("TableName: got %q", op.TableName())
	}
}

// TestUpsertData_IsDestructive verifies the operation is not marked destructive.
func TestUpsertData_IsDestructive(t *testing.T) {
	op := &migrate.UpsertData{}
	if op.IsDestructive() {
		t.Error("expected IsDestructive = false")
	}
}

// TestUpsertData_Describe verifies the human-readable description.
func TestUpsertData_Describe(t *testing.T) {
	op := &migrate.UpsertData{
		Table: "countries",
		Rows:  []map[string]any{{"code": "AU"}, {"code": "US"}},
	}
	desc := op.Describe()
	if !strings.Contains(desc, "2") || !strings.Contains(desc, "countries") {
		t.Errorf("Describe: got %q", desc)
	}
}

// TestUpsertData_Mutate verifies Mutate is a no-op that does not error.
func TestUpsertData_Mutate(t *testing.T) {
	op := &migrate.UpsertData{}
	if err := op.Mutate(nil); err != nil {
		t.Errorf("Mutate returned error: %v", err)
	}
}

// TestUpsertData_Up_Empty verifies that an empty Rows slice produces empty SQL.
func TestUpsertData_Up_Empty(t *testing.T) {
	op := &migrate.UpsertData{Table: "t", ConflictKeys: []string{"id"}, Rows: nil}
	p := postgresql.New()
	sql, err := op.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty SQL for empty rows, got: %q", sql)
	}
}

// TestUpsertData_Up_PostgreSQL verifies the PostgreSQL ON CONFLICT upsert output.
func TestUpsertData_Up_PostgreSQL(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		Rows: []map[string]any{
			{"code": "AU", "name": "Australia"},
			{"code": "US", "name": "United States"},
		},
	}
	p := postgresql.New()
	sql, err := op.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("expected ON CONFLICT in PostgreSQL upsert, got:\n%s", sql)
	}
	if !strings.Contains(sql, "'AU'") || !strings.Contains(sql, "'US'") {
		t.Errorf("expected row values in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "countries") {
		t.Errorf("expected table name in SQL, got:\n%s", sql)
	}
}

// TestUpsertData_Up_SQLite verifies the SQLite ON CONFLICT upsert output.
func TestUpsertData_Up_SQLite(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "statuses",
		ConflictKeys: []string{"id"},
		Rows: []map[string]any{
			{"id": 1, "label": "active"},
			{"id": 2, "label": "inactive"},
		},
	}
	p := sqlite.New()
	sql, err := op.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("expected ON CONFLICT in SQLite upsert, got:\n%s", sql)
	}
	if !strings.Contains(sql, "excluded") {
		t.Errorf("expected 'excluded' alias in SQLite upsert, got:\n%s", sql)
	}
}

// TestUpsertData_Down_GeneratesDeletes verifies rollback produces DELETE statements.
func TestUpsertData_Down_PostgreSQL(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		Rows: []map[string]any{
			{"code": "AU", "name": "Australia"},
			{"code": "US", "name": "United States"},
		},
	}
	p := postgresql.New()
	sql, err := op.Down(p, nil, nil)
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}
	if !strings.Contains(sql, "DELETE FROM") {
		t.Errorf("expected DELETE FROM in rollback SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "'AU'") || !strings.Contains(sql, "'US'") {
		t.Errorf("expected conflict key values in DELETE, got:\n%s", sql)
	}
}

// TestUpsertData_Down_EmptyRows verifies no SQL generated when rows is empty.
func TestUpsertData_Down_EmptyRows(t *testing.T) {
	op := &migrate.UpsertData{Table: "t", ConflictKeys: []string{"id"}, Rows: nil}
	p := postgresql.New()
	sql, err := op.Down(p, nil, nil)
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty SQL for empty rows, got: %q", sql)
	}
}

// TestUpsertData_Down_NoConflictKeys verifies no SQL when no conflict keys.
func TestUpsertData_Down_NoConflictKeys(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "t",
		ConflictKeys: []string{},
		Rows:         []map[string]any{{"a": "b"}},
	}
	p := postgresql.New()
	sql, err := op.Down(p, nil, nil)
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty SQL with no conflict keys, got: %q", sql)
	}
}

// TestUpsertData_Up_DefaultRef_Resolved verifies that DefaultRef values are
// replaced with the resolved SQL expression from the defaults map.
func TestUpsertData_Up_DefaultRef_Resolved(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "items",
		ConflictKeys: []string{"code"},
		Rows: []map[string]any{
			{"code": "AU", "id": migrate.DefaultRef("uuid"), "name": "Australia"},
		},
	}
	p := postgresql.New()
	defaults := map[string]string{"uuid": "uuid_generate_v4()"}
	sql, err := op.Up(p, nil, defaults)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	// The resolved expression must appear unquoted.
	if !strings.Contains(sql, "uuid_generate_v4()") {
		t.Errorf("expected resolved default expression in SQL, got:\n%s", sql)
	}
	// It must NOT appear as a quoted string literal.
	if strings.Contains(sql, "'uuid'") {
		t.Errorf("DefaultRef key should not be quoted as a string literal, got:\n%s", sql)
	}
}

// TestUpsertData_Up_DefaultRef_Fallback verifies that an unresolved DefaultRef
// key is emitted as a raw SQL expression (not quoted).
func TestUpsertData_Up_DefaultRef_Fallback(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "items",
		ConflictKeys: []string{"code"},
		Rows: []map[string]any{
			{"code": "AU", "ts": migrate.DefaultRef("NOW()")},
		},
	}
	p := postgresql.New()
	sql, err := op.Up(p, nil, nil) // no defaults map
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	if !strings.Contains(sql, "NOW()") {
		t.Errorf("expected raw SQL expression fallback, got:\n%s", sql)
	}
	if strings.Contains(sql, "'NOW()'") {
		t.Errorf("DefaultRef fallback should not be quoted, got:\n%s", sql)
	}
}

// TestUpsertData_Up_DefaultRef_NilDefaults verifies behaviour when defaults is nil.
func TestUpsertData_Up_DefaultRef_NilDefaults(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "t",
		ConflictKeys: []string{"id"},
		Rows: []map[string]any{
			{"id": migrate.DefaultRef("uuid"), "val": "x"},
		},
	}
	p := postgresql.New()
	// Must not panic with nil defaults.
	sql, err := op.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	// Fallback: "uuid" emitted as raw expression.
	if !strings.Contains(sql, "uuid") {
		t.Errorf("expected 'uuid' in SQL fallback, got:\n%s", sql)
	}
}

// TestUpsertData_Up_ColumnOrder verifies columns are sorted alphabetically.
func TestUpsertData_Up_ColumnOrder(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "t",
		ConflictKeys: []string{"z_key"},
		Rows: []map[string]any{
			{"z_key": "k1", "a_col": "v1", "m_col": "v2"},
		},
	}
	p := postgresql.New()
	sql, err := op.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("Up error: %v", err)
	}
	// "a_col" should appear before "m_col" should appear before "z_key"
	posA := strings.Index(sql, "a_col")
	posM := strings.Index(sql, "m_col")
	posZ := strings.Index(sql, "z_key")
	if posA >= posM || posM >= posZ {
		t.Errorf("expected alphabetical column order in SQL:\n%s", sql)
	}
}

// TestUpsertData_Describe_WithDataFile verifies the description when DataFile
// is set and Rows is nil.
func TestUpsertData_Describe_WithDataFile(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		DataFile:     "data/x.jsonl",
	}
	want := "Upsert rows from data/x.jsonl into countries"
	if got := op.Describe(); got != want {
		t.Errorf("Describe: got %q, want %q", got, want)
	}
}

// TestUpsertData_DataFileResolver_Interface verifies that *UpsertData
// implements the DataFileResolver interface.
func TestUpsertData_DataFileResolver_Interface(t *testing.T) {
	var _ migrate.DataFileResolver = (*migrate.UpsertData)(nil)
}

// TestUpsertData_Up_WithDataFile creates a temp JSONL file, wires up
// UpsertData via SetBasePath, and verifies Up produces the same SQL as
// an inline-equivalent operation.
func TestUpsertData_Up_WithDataFile(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonlPath := filepath.Join(dataDir, "countries.jsonl")
	content := `{"code":"AU","name":"Australia"}
{"code":"US","name":"United States"}
`
	if err := os.WriteFile(jsonlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	// File-backed operation.
	fileBacked := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		DataFile:     "data/countries.jsonl",
	}
	fileBacked.SetBasePath(dir)

	// Inline equivalent.
	inline := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		Rows: []map[string]any{
			{"code": "AU", "name": "Australia"},
			{"code": "US", "name": "United States"},
		},
	}

	p := postgresql.New()
	sqlFile, err := fileBacked.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("file-backed Up error: %v", err)
	}
	sqlInline, err := inline.Up(p, nil, nil)
	if err != nil {
		t.Fatalf("inline Up error: %v", err)
	}
	if sqlFile != sqlInline {
		t.Errorf("SQL mismatch.\nfile-backed:\n%s\ninline:\n%s", sqlFile, sqlInline)
	}
}

// TestUpsertData_Down_WithDataFile verifies that Down with a DataFile
// produces the same DELETE statements as inline rows.
func TestUpsertData_Down_WithDataFile(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "countries.jsonl")
	content := `{"code":"AU","name":"Australia"}
{"code":"US","name":"United States"}
`
	if err := os.WriteFile(jsonlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	fileBacked := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		DataFile:     "countries.jsonl",
	}
	fileBacked.SetBasePath(dir)

	p := postgresql.New()
	sql, err := fileBacked.Down(p, nil, nil)
	if err != nil {
		t.Fatalf("Down error: %v", err)
	}
	if !strings.Contains(sql, "DELETE FROM") {
		t.Errorf("expected DELETE FROM in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "'AU'") || !strings.Contains(sql, "'US'") {
		t.Errorf("expected conflict key values in DELETE, got:\n%s", sql)
	}
}

// TestUpsertData_Up_DataFileMissing verifies that a nonexistent DataFile
// produces an error.
func TestUpsertData_Up_DataFileMissing(t *testing.T) {
	op := &migrate.UpsertData{
		Table:        "countries",
		ConflictKeys: []string{"code"},
		DataFile:     "nonexistent.jsonl",
	}
	op.SetBasePath(t.TempDir())

	p := postgresql.New()
	_, err := op.Up(p, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing data file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}
