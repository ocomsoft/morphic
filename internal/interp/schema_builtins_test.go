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

package interp

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/ocomsoft/morphic/internal/types"
)

// execSchemaDSL is a test helper that executes a Starlark schema script and
// returns the collected types.Schema.
func execSchemaDSL(t *testing.T, src string) *types.Schema {
	t.Helper()
	b := NewSchemaDSLBuiltins()
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", src, b.Env())
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	return b.Collected()
}

// execSchemaDSLErr is a test helper that executes a Starlark schema script and
// expects an error. Returns the error message.
func execSchemaDSLErr(t *testing.T, src string) string {
	t.Helper()
	b := NewSchemaDSLBuiltins()
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", src, b.Env())
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
	return err.Error()
}

func TestSchemaDSL_SimpleSchema(t *testing.T) {
	src := `
database("myapp", "1.0.0")
table("users",
    fields = [
        serial("id", primary_key=True),
        varchar("name", 255),
    ],
)
`
	schema := execSchemaDSL(t, src)

	if schema.Database.Name != "myapp" {
		t.Errorf("database name = %q, want %q", schema.Database.Name, "myapp")
	}
	if schema.Database.Version != "1.0.0" {
		t.Errorf("database version = %q, want %q", schema.Database.Version, "1.0.0")
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(schema.Tables))
	}
	tbl := schema.Tables[0]
	if tbl.Name != "users" {
		t.Errorf("table name = %q, want %q", tbl.Name, "users")
	}
	if len(tbl.Fields) != 2 {
		t.Fatalf("got %d fields, want 2", len(tbl.Fields))
	}
	if tbl.Fields[0].Type != "serial" || !tbl.Fields[0].PrimaryKey {
		t.Errorf("first field: type=%q pk=%v, want serial/true", tbl.Fields[0].Type, tbl.Fields[0].PrimaryKey)
	}
	if tbl.Fields[1].Type != "varchar" || tbl.Fields[1].Length != 255 {
		t.Errorf("second field: type=%q length=%d, want varchar/255", tbl.Fields[1].Type, tbl.Fields[1].Length)
	}
}

func TestSchemaDSL_AllFieldTypes(t *testing.T) {
	src := `
database("test", "1.0.0")
table("all_types",
    fields = [
        serial("id", primary_key=True),
        uuid("uid"),
        varchar("name", 100),
        text("bio"),
        integer("age"),
        bigint("big_num"),
        boolean("active"),
        timestamp("created_at"),
        date("birth_date"),
        time("start_time"),
        float("score"),
        jsonb("metadata"),
        bytes("data"),
        decimal("price", 10, 2),
        foreign_key("user_id", fk("users", on_delete="CASCADE")),
    ],
)
`
	schema := execSchemaDSL(t, src)

	if len(schema.Tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(schema.Tables))
	}

	expectedTypes := []string{
		"serial", "uuid", "varchar", "text", "integer", "bigint",
		"boolean", "timestamp", "date", "time", "float", "jsonb",
		"bytes", "decimal", "foreign_key",
	}

	tbl := schema.Tables[0]
	if len(tbl.Fields) != len(expectedTypes) {
		t.Fatalf("got %d fields, want %d", len(tbl.Fields), len(expectedTypes))
	}
	for i, ft := range expectedTypes {
		if tbl.Fields[i].Type != ft {
			t.Errorf("field %d: type=%q, want %q", i, tbl.Fields[i].Type, ft)
		}
	}

	// Check decimal specifics.
	decField := tbl.Fields[13]
	if decField.Precision != 10 || decField.Scale != 2 {
		t.Errorf("decimal: precision=%d scale=%d, want 10/2", decField.Precision, decField.Scale)
	}

	// Check foreign key specifics.
	fkField := tbl.Fields[14]
	if fkField.ForeignKey == nil {
		t.Fatal("foreign_key field has nil ForeignKey")
	}
	if fkField.ForeignKey.Table != "users" || fkField.ForeignKey.OnDelete != "CASCADE" {
		t.Errorf("fk: table=%q on_delete=%q, want users/CASCADE",
			fkField.ForeignKey.Table, fkField.ForeignKey.OnDelete)
	}
}

func TestSchemaDSL_DefaultsAndTypeMappings(t *testing.T) {
	src := `
database("test", "1.0.0")
defaults("postgresql", {"blank": "''", "now": "CURRENT_TIMESTAMP"})
defaults("mysql", {"blank": "''", "now": "NOW()"})
type_mappings("postgresql", {"money": "NUMERIC(19,4)"})
table("t",
    fields = [
        integer("id", primary_key=True),
    ],
)
`
	schema := execSchemaDSL(t, src)

	pgDefaults := schema.Defaults.ForProvider(types.DatabasePostgreSQL)
	if pgDefaults == nil {
		t.Fatal("no postgresql defaults")
	}
	if pgDefaults["blank"] != "''" {
		t.Errorf("pg blank = %q, want %q", pgDefaults["blank"], "''")
	}
	if pgDefaults["now"] != "CURRENT_TIMESTAMP" {
		t.Errorf("pg now = %q, want %q", pgDefaults["now"], "CURRENT_TIMESTAMP")
	}

	myDefaults := schema.Defaults.ForProvider(types.DatabaseMySQL)
	if myDefaults == nil {
		t.Fatal("no mysql defaults")
	}
	if myDefaults["now"] != "NOW()" {
		t.Errorf("mysql now = %q, want %q", myDefaults["now"], "NOW()")
	}

	pgMappings := schema.TypeMappings.ForProvider(types.DatabasePostgreSQL)
	if pgMappings == nil {
		t.Fatal("no postgresql type mappings")
	}
	if pgMappings["money"] != "NUMERIC(19,4)" {
		t.Errorf("pg money = %q, want %q", pgMappings["money"], "NUMERIC(19,4)")
	}
}

func TestSchemaDSL_ForeignKeysAndIndexes(t *testing.T) {
	src := `
database("test", "1.0.0")
table("posts",
    fields = [
        serial("id", primary_key=True),
        foreign_key("author_id", fk("users", on_delete="CASCADE", on_update="SET NULL")),
        varchar("title", 200),
    ],
    indexes = [
        index("idx_posts_author", ["author_id"]),
        index("idx_posts_title", ["title"], unique=True, method="btree"),
    ],
)
`
	schema := execSchemaDSL(t, src)

	tbl := schema.Tables[0]

	// Check FK.
	fkField := tbl.Fields[1]
	if fkField.ForeignKey == nil {
		t.Fatal("author_id has nil ForeignKey")
	}
	if fkField.ForeignKey.Table != "users" {
		t.Errorf("fk table = %q, want %q", fkField.ForeignKey.Table, "users")
	}
	if fkField.ForeignKey.OnDelete != "CASCADE" {
		t.Errorf("fk on_delete = %q, want %q", fkField.ForeignKey.OnDelete, "CASCADE")
	}
	if fkField.ForeignKey.OnUpdate != "SET NULL" {
		t.Errorf("fk on_update = %q, want %q", fkField.ForeignKey.OnUpdate, "SET NULL")
	}

	// Check indexes.
	if len(tbl.Indexes) != 2 {
		t.Fatalf("got %d indexes, want 2", len(tbl.Indexes))
	}
	if tbl.Indexes[0].Name != "idx_posts_author" {
		t.Errorf("index 0 name = %q", tbl.Indexes[0].Name)
	}
	if tbl.Indexes[1].Unique != true {
		t.Error("index 1 should be unique")
	}
	if tbl.Indexes[1].Method != "btree" {
		t.Errorf("index 1 method = %q, want %q", tbl.Indexes[1].Method, "btree")
	}
}

func TestSchemaDSL_IncludeEntries(t *testing.T) {
	src := `
database("test", "1.0.0")
include("auth", "schemas/auth/schema.yaml")
include("billing", "schemas/billing/schema.yaml")
table("t",
    fields = [
        integer("id", primary_key=True),
    ],
)
`
	schema := execSchemaDSL(t, src)

	if len(schema.Include) != 2 {
		t.Fatalf("got %d includes, want 2", len(schema.Include))
	}
	if schema.Include[0].Module != "auth" || schema.Include[0].Path != "schemas/auth/schema.yaml" {
		t.Errorf("include 0 = %+v", schema.Include[0])
	}
	if schema.Include[1].Module != "billing" || schema.Include[1].Path != "schemas/billing/schema.yaml" {
		t.Errorf("include 1 = %+v", schema.Include[1])
	}
}

func TestSchemaDSL_ErrorMissingDatabaseName(t *testing.T) {
	src := `database("", "1.0.0")`
	errMsg := execSchemaDSLErr(t, src)
	if !strings.Contains(errMsg, "name must not be empty") {
		t.Errorf("unexpected error: %s", errMsg)
	}
}

func TestSchemaDSL_ErrorTableFieldNotDict(t *testing.T) {
	src := `
database("test", "1.0.0")
table("bad", fields = ["not_a_dict"])
`
	errMsg := execSchemaDSLErr(t, src)
	if !strings.Contains(errMsg, "must be a dict") {
		t.Errorf("unexpected error: %s", errMsg)
	}
}
