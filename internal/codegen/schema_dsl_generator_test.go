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

package codegen

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/types"
)

// boolPtr is a helper that returns a pointer to a bool value.
func boolPtr(b bool) *bool { return &b }

func TestGenerateSchemaDSL_RoundTrip(t *testing.T) {
	// Build a schema programmatically.
	original := &types.Schema{
		Database: types.Database{
			Name:    "myapp",
			Version: "2.0.0",
		},
		Include: []types.Include{
			{Module: "auth", Path: "schemas/auth/schema.yaml"},
		},
		Defaults: types.Defaults{
			types.DatabasePostgreSQL: {"blank": "''", "now": "CURRENT_TIMESTAMP"},
		},
		TypeMappings: types.TypeMappings{
			types.DatabasePostgreSQL: {"money": "NUMERIC(19,4)"},
		},
		Tables: []types.Table{
			{
				Name: "users",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255, Nullable: boolPtr(true)},
					{Name: "bio", Type: "text", Nullable: boolPtr(true)},
					{Name: "active", Type: "boolean"},
					{Name: "score", Type: "decimal", Precision: 10, Scale: 2},
				},
				Indexes: []types.Index{
					{Name: "idx_users_email", Fields: []string{"email"}, Unique: true},
				},
			},
			{
				Name: "posts",
				Fields: []types.Field{
					{Name: "id", Type: "serial", PrimaryKey: true},
					{Name: "author_id", Type: "foreign_key", ForeignKey: &types.ForeignKey{
						Table: "users", OnDelete: "CASCADE",
					}},
					{Name: "title", Type: "varchar", Length: 200},
				},
			},
		},
	}

	// Generate DSL.
	dsl, err := GenerateSchemaDSL(original)
	if err != nil {
		t.Fatalf("GenerateSchemaDSL: %v", err)
	}

	// Verify the output contains expected tokens.
	checks := []string{
		`database("myapp", "2.0.0")`,
		`include("auth", "schemas/auth/schema.yaml")`,
		`defaults("postgresql"`,
		`type_mappings("postgresql"`,
		`table("users"`,
		`table("posts"`,
		`serial("id"`,
		`varchar("email", 255`,
		`decimal("score", 10, 2`,
		`foreign_key("author_id"`,
		`index("idx_users_email"`,
	}
	for _, check := range checks {
		if !strings.Contains(dsl, check) {
			t.Errorf("generated DSL missing %q\n\nFull output:\n%s", check, dsl)
		}
	}

	// Parse the generated DSL back and compare key properties.
	builtins := interp.NewSchemaDSLBuiltins()
	thread := &starlark.Thread{Name: "roundtrip"}
	_, err = starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", dsl, builtins.Env())
	if err != nil {
		t.Fatalf("re-parse failed: %v\n\nGenerated DSL:\n%s", err, dsl)
	}

	parsed := builtins.Collected()

	if parsed.Database.Name != original.Database.Name {
		t.Errorf("roundtrip db name = %q, want %q", parsed.Database.Name, original.Database.Name)
	}
	if parsed.Database.Version != original.Database.Version {
		t.Errorf("roundtrip db version = %q, want %q", parsed.Database.Version, original.Database.Version)
	}
	if len(parsed.Tables) != len(original.Tables) {
		t.Fatalf("roundtrip tables = %d, want %d", len(parsed.Tables), len(original.Tables))
	}

	// Check table names and field counts.
	for i, orig := range original.Tables {
		p := parsed.Tables[i]
		if p.Name != orig.Name {
			t.Errorf("roundtrip table %d name = %q, want %q", i, p.Name, orig.Name)
		}
		if len(p.Fields) != len(orig.Fields) {
			t.Errorf("roundtrip table %q fields = %d, want %d", orig.Name, len(p.Fields), len(orig.Fields))
		}
	}

	// Check includes.
	if len(parsed.Include) != len(original.Include) {
		t.Errorf("roundtrip includes = %d, want %d", len(parsed.Include), len(original.Include))
	}

	// Check defaults round-tripped.
	pgDefaults := parsed.Defaults.ForProvider(types.DatabasePostgreSQL)
	if pgDefaults == nil || pgDefaults["now"] != "CURRENT_TIMESTAMP" {
		t.Errorf("roundtrip pg defaults missing or wrong: %v", pgDefaults)
	}

	// Check type mappings round-tripped.
	pgMappings := parsed.TypeMappings.ForProvider(types.DatabasePostgreSQL)
	if pgMappings == nil || pgMappings["money"] != "NUMERIC(19,4)" {
		t.Errorf("roundtrip pg type_mappings missing or wrong: %v", pgMappings)
	}
}

func TestGenerateSchemaDSL_NilSchema(t *testing.T) {
	_, err := GenerateSchemaDSL(nil)
	if err == nil {
		t.Fatal("expected error for nil schema")
	}
}
