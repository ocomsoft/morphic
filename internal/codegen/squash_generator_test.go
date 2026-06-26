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

package codegen_test

import (
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/migrate"
)

func TestSquashGenerator_GenerateStarlarkSquash(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name:         "0001_initial",
			Dependencies: []string{},
			Operations: []migrate.Operation{
				&migrate.CreateTable{
					Name:   "users",
					Fields: []migrate.Field{{Name: "id", Type: "uuid", PrimaryKey: true}},
				},
			},
		},
		{
			Name:         "0002_add_phone",
			Dependencies: []string{"0001_initial"},
			Operations: []migrate.Operation{
				&migrate.AddField{
					Table: "users",
					Field: migrate.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: true},
				},
			},
		},
	}

	g := codegen.NewSquashGenerator()
	src, err := g.GenerateStarlarkSquash("0001_squashed_0002", []string{"0001_initial", "0002_add_phone"}, migrations)
	if err != nil {
		t.Fatalf("GenerateStarlarkSquash: %v", err)
	}

	if !strings.Contains(src, "migration(") {
		t.Error("expected migration( in output")
	}
	if !strings.Contains(src, `name = "0001_squashed_0002"`) {
		t.Error("expected squash name in output")
	}
	if !strings.Contains(src, "replaces = [") {
		t.Error("expected replaces field in output")
	}
	if !strings.Contains(src, `"0001_initial"`) {
		t.Error("expected replaced migration in replaces list")
	}
	if !strings.Contains(src, "create_table(") {
		t.Error("expected create_table operation in Starlark output")
	}
	if !strings.Contains(src, "add_field(") {
		t.Error("expected add_field operation in Starlark output")
	}
	if strings.Contains(src, "package main") {
		t.Error("Starlark output must not contain 'package main'")
	}
}

func TestSquashGenerator_GenerateStarlarkSquash_AllOpTypes(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name:         "0001_all_ops",
			Dependencies: []string{},
			Operations: []migrate.Operation{
				&migrate.CreateTable{Name: "tbl", Fields: []migrate.Field{{Name: "id", Type: "integer"}}},
				&migrate.AddField{Table: "tbl", Field: migrate.Field{Name: "col", Type: "text"}},
				&migrate.AddIndex{Table: "tbl", Index: migrate.Index{Name: "idx_col", Fields: []string{"col"}, Unique: true}},
				&migrate.RunSQL{ForwardSQL: "SELECT 1", BackwardSQL: "SELECT 0"},
			},
		},
	}

	g := codegen.NewSquashGenerator()
	src, err := g.GenerateStarlarkSquash("0002_squash", []string{"0001_all_ops"}, migrations)
	if err != nil {
		t.Fatalf("GenerateStarlarkSquash: %v", err)
	}

	if !strings.Contains(src, "add_field(") {
		t.Error("expected add_field in output")
	}
	if !strings.Contains(src, "add_index(") {
		t.Error("expected add_index in output")
	}
	if !strings.Contains(src, "run_sql(") {
		t.Error("expected run_sql in output")
	}
}

func TestSquashGenerator_GenerateStarlarkSquash_EmptyMigrations(t *testing.T) {
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateStarlarkSquash("0001_squash", []string{}, []*migrate.Migration{})
	if err != nil {
		t.Fatalf("GenerateStarlarkSquash with empty migrations: %v", err)
	}
	if !strings.Contains(src, "operations = [") {
		t.Error("expected operations field in output")
	}
}
