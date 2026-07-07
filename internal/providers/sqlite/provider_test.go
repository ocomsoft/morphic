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
package sqlite

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"github.com/ocomsoft/morphic/internal/types"
)

func TestProvider_IsNotFoundError(t *testing.T) {
	p := New()
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("no such table: users"), true},
		{errors.New("no such column: email"), true},
		{errors.New("no such index: idx_email"), true},
		{errors.New("UNIQUE constraint failed: users.email"), false},
		{errors.New("connection refused"), false},
		{nil, false},
	}
	for _, tc := range cases {
		got := p.IsNotFoundError(tc.err)
		if got != tc.want {
			t.Errorf("IsNotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestProvider_GenerateDropColumn(t *testing.T) {
	p := New()
	got := p.GenerateDropColumn("orders", "notes")
	want := `ALTER TABLE "orders" DROP COLUMN "notes";`
	if got != want {
		t.Errorf("GenerateDropColumn() = %q, want %q", got, want)
	}
}

func TestProvider_GenerateRenameColumn(t *testing.T) {
	p := New()
	got := p.GenerateRenameColumn("users", "phone_number", "phone")
	want := `ALTER TABLE "users" RENAME COLUMN "phone_number" TO "phone";`
	if got != want {
		t.Errorf("GenerateRenameColumn() = %q, want %q", got, want)
	}
}

func TestProvider_GenerateAlterColumnWithTable_TableRecreation(t *testing.T) {
	p := New()
	f := boolPtr(false)
	currentTable := &types.Table{
		Name: "users",
		Fields: []types.Field{
			{Name: "id", Type: "integer", PrimaryKey: true},
			{Name: "email", Type: "varchar"},
			{Name: "phone", Type: "varchar", Nullable: nil}, // currently nullable (nil = nullable)
		},
	}
	from := &types.Field{Name: "phone", Type: "varchar", Nullable: nil} // nullable
	to := &types.Field{Name: "phone", Type: "varchar", Nullable: f}     // NOT NULL

	got, err := p.GenerateAlterColumnWithTable(currentTable, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty SQL for table recreation, got empty string")
	}
	if !strings.Contains(got, "users__migration") {
		t.Errorf("expected temp table 'users__migration' in SQL:\n%s", got)
	}
	if !strings.Contains(got, "INSERT INTO") {
		t.Errorf("expected INSERT INTO in SQL:\n%s", got)
	}
	if !strings.Contains(got, `DROP TABLE "users"`) {
		t.Errorf("expected DROP TABLE \"users\" in SQL:\n%s", got)
	}
	if !strings.Contains(got, `RENAME TO "users"`) {
		t.Errorf("expected RENAME TO \"users\" in SQL:\n%s", got)
	}
}

func TestProvider_GenerateAlterColumnWithTable_NoChange(t *testing.T) {
	p := New()
	currentTable := &types.Table{
		Name:   "users",
		Fields: []types.Field{{Name: "score", Type: "integer"}},
	}
	from := &types.Field{Name: "score", Type: "integer"}
	to := &types.Field{Name: "score", Type: "integer"}
	got, err := p.GenerateAlterColumnWithTable(currentTable, from, to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty SQL for no-change alter, got: %q", got)
	}
}

// boolPtr converts a bool to a *bool for use with types.Field.Nullable.
func boolPtr(b bool) *bool { return &b }

// TestProvider_GenerateAlterColumn_NoOp verifies that GenerateAlterColumn returns
func TestProvider_GenerateAlterColumn_NoOp(t *testing.T) {
	p := New()
	old := &types.Field{Name: "score", Type: "integer"}
	nw := &types.Field{Name: "score", Type: "text"}
	sql, err := p.GenerateAlterColumn("results", old, nw)
	if err != nil {
		t.Fatalf("expected no error for ALTER COLUMN on SQLite, got: %v", err)
	}
	if sql != "" {
		t.Errorf("expected empty SQL for unsupported ALTER COLUMN, got: %q", sql)
	}
}

func TestProvider_GenerateAlterColumn_NoChange(t *testing.T) {
	p := New()
	old := &types.Field{Name: "name", Type: "varchar", Length: 100}
	nw := &types.Field{Name: "name", Type: "varchar", Length: 100}
	got, err := p.GenerateAlterColumn("things", old, nw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for no-change alter, got: %q", got)
	}
}

func TestProvider_GenerateForeignKeyConstraints(t *testing.T) {
	p := New()
	schema := &types.Schema{
		Tables: []types.Table{
			{
				Name: "orders",
				Fields: []types.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "user_id", Type: "foreign_key", ForeignKey: &types.ForeignKey{Table: "users", OnDelete: "cascade"}},
				},
			},
		},
	}
	got := p.GenerateForeignKeyConstraints(schema, nil)
	// SQLite's GenerateForeignKeyConstraint returns "", so this should also be empty
	if got != "" {
		t.Errorf("expected empty for SQLite FK constraints, got:\n%s", got)
	}
}

func TestProvider_GenerateForeignKeyConstraints_Empty(t *testing.T) {
	p := New()
	schema := &types.Schema{
		Tables: []types.Table{
			{
				Name: "simple",
				Fields: []types.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	}
	got := p.GenerateForeignKeyConstraints(schema, nil)
	if got != "" {
		t.Errorf("expected empty for no FKs, got:\n%s", got)
	}
}

func TestProvider_InferForeignKeyType(t *testing.T) {
	provider := New()

	result := provider.InferForeignKeyType("users", &types.Schema{})
	expected := "INTEGER"

	if result != expected {
		t.Errorf("InferForeignKeyType() = %s; expected %s", result, expected)
	}
}

func TestProvider_GenerateIndexes(t *testing.T) {
	p := New()
	schema := &types.Schema{
		Tables: []types.Table{
			{
				Name: "orders",
				Fields: []types.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "user_id", Type: "foreign_key"},
				},
			},
		},
	}
	got := p.GenerateIndexes(schema)
	if got == "" {
		t.Error("expected non-empty indexes output")
	}
	if !strings.Contains(got, "idx_orders_user_id") {
		t.Errorf("expected FK index in:\n%s", got)
	}
}

func TestProvider_GenerateIndexes_Empty(t *testing.T) {
	p := New()
	schema := &types.Schema{
		Tables: []types.Table{
			{
				Name: "simple",
				Fields: []types.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "name", Type: "varchar"},
				},
			},
		},
	}
	got := p.GenerateIndexes(schema)
	if got != "" {
		t.Errorf("expected empty indexes for schema without FKs, got:\n%s", got)
	}
}

func TestProvider_GenerateAddColumn_NoDefault(t *testing.T) {
	p := New()
	field := types.Field{Name: "score", Type: "integer"}
	field.SetNullable(false)
	got := p.GenerateAddColumn("items", &field)
	expected := `ALTER TABLE "items" ADD COLUMN "score" INTEGER NOT NULL;`
	if got != expected {
		t.Errorf("GenerateAddColumn() = %q; want %q", got, expected)
	}
}

func TestProvider_GenerateAddColumn_WithDefault(t *testing.T) {
	p := New()
	field := types.Field{Name: "display_order", Type: "integer", Default: "0"}
	field.SetNullable(false)
	got := p.GenerateAddColumn("items", &field)
	expected := `ALTER TABLE "items" ADD COLUMN "display_order" INTEGER NOT NULL DEFAULT 0;`
	if got != expected {
		t.Errorf("GenerateAddColumn() = %q; want %q", got, expected)
	}
}

func TestProvider_GenerateCreateIndex_WithWhere(t *testing.T) {
	p := New()
	idx := &types.Index{Name: "u_email_idx", Fields: []string{"email"}, Where: "deleted_at IS NULL"}
	sql := p.GenerateCreateIndex(idx, "users")
	if !strings.Contains(sql, "WHERE deleted_at IS NULL") {
		t.Errorf("expected WHERE clause, got: %s", sql)
	}
}

func TestProvider_GenerateAddColumn_PrimaryKey(t *testing.T) {
	p := New()
	field := types.Field{Name: "id", Type: "uuid", PrimaryKey: true}
	field.SetNullable(false)
	got := p.GenerateAddColumn("users", &field)
	if !strings.Contains(got, "PRIMARY KEY") {
		t.Errorf("GenerateAddColumn() with PrimaryKey=true should contain PRIMARY KEY, got: %s", got)
	}
	if !strings.Contains(got, `"id"`) {
		t.Errorf("GenerateAddColumn() should contain quoted field name, got: %s", got)
	}
}

func TestTableColumns_ExistingTable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT, name TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	p := New()
	cols, err := p.TableColumns(db, "users")
	if err != nil {
		t.Fatalf("TableColumns: %v", err)
	}

	expected := []string{"id", "email", "name"}
	if len(cols) != len(expected) {
		t.Fatalf("expected %d columns, got %d: %v", len(expected), len(cols), cols)
	}
	for i, col := range cols {
		if col != expected[i] {
			t.Errorf("column %d: expected %q, got %q", i, expected[i], col)
		}
	}
}

func TestTableColumns_NonExistentTable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	p := New()
	cols, err := p.TableColumns(db, "nonexistent")
	if err != nil {
		t.Fatalf("TableColumns should not error for missing table: %v", err)
	}
	if cols != nil {
		t.Errorf("expected nil for non-existent table, got %v", cols)
	}
}
