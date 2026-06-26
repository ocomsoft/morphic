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
	"go/format"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/migrate"
)

func TestSquashGenerator_GenerateSquash(t *testing.T) {
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
	src, err := g.GenerateSquash("0001_squashed_0002", []string{"0001_initial", "0002_add_phone"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "0001_squashed_0002") {
		t.Error("expected squash name in output")
	}
	if !strings.Contains(src, "Replaces") {
		t.Error("expected Replaces field in output")
	}
	if !strings.Contains(src, "0001_initial") {
		t.Error("expected replaced migration in Replaces list")
	}
	if !strings.Contains(src, "CreateTable") {
		t.Error("expected CreateTable operation in squashed output")
	}
}

func TestSquashGenerator_GenerateSquash_AllOpTypes(t *testing.T) {
	// Test that all operation types are correctly rendered
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
	src, err := g.GenerateSquash("0002_squash", []string{"0001_all_ops"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "AddField") {
		t.Error("expected AddField in output")
	}
	if !strings.Contains(src, "AddIndex") {
		t.Error("expected AddIndex in output")
	}
	if !strings.Contains(src, "RunSQL") {
		t.Error("expected RunSQL in output")
	}
}

func TestSquashGenerator_GenerateSquash_EmptyMigrations(t *testing.T) {
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{}, []*migrate.Migration{})
	if err != nil {
		t.Fatalf("GenerateSquash with empty migrations: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v", err)
	}
}

func TestSquashGenerator_GenerateSquash_DropTable(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_drop",
			Operations: []migrate.Operation{
				&migrate.DropTable{Name: "old_table"},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_drop"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "DropTable") {
		t.Error("expected DropTable in output")
	}
	if !strings.Contains(src, `"old_table"`) {
		t.Error("expected table name in output")
	}
}

func TestSquashGenerator_GenerateSquash_RenameTable(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_rename",
			Operations: []migrate.Operation{
				&migrate.RenameTable{OldName: "users", NewName: "accounts"},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_rename"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "RenameTable") {
		t.Error("expected RenameTable in output")
	}
	if !strings.Contains(src, `"accounts"`) {
		t.Error("expected new table name in output")
	}
}

func TestSquashGenerator_GenerateSquash_DropField(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_drop_field",
			Operations: []migrate.Operation{
				&migrate.DropField{Table: "users", Field: "phone"},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_drop_field"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "DropField") {
		t.Error("expected DropField in output")
	}
}

func TestSquashGenerator_GenerateSquash_RenameField(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_rename_field",
			Operations: []migrate.Operation{
				&migrate.RenameField{Table: "users", OldName: "email", NewName: "email_address"},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_rename_field"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "RenameField") {
		t.Error("expected RenameField in output")
	}
	if !strings.Contains(src, `"email_address"`) {
		t.Error("expected new field name in output")
	}
}

func TestSquashGenerator_GenerateSquash_AlterField(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_alter",
			Operations: []migrate.Operation{
				&migrate.AlterField{
					Table:    "users",
					OldField: migrate.Field{Name: "email", Type: "varchar", Length: 100},
					NewField: migrate.Field{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_alter"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "AlterField") {
		t.Error("expected AlterField in output")
	}
	if !strings.Contains(src, "Length: 255") {
		t.Error("expected Length: 255 in new field")
	}
	if !strings.Contains(src, "Length: 100") {
		t.Error("expected Length: 100 in old field")
	}
}

func TestSquashGenerator_GenerateSquash_DropIndex(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_drop_idx",
			Operations: []migrate.Operation{
				&migrate.DropIndex{Table: "users", Index: "idx_users_email"},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_drop_idx"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "DropIndex") {
		t.Error("expected DropIndex in output")
	}
	if !strings.Contains(src, `"idx_users_email"`) {
		t.Error("expected index name in output")
	}
}

func TestSquashGenerator_GenerateSquash_CreateTableWithIndexes(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_with_idx",
			Operations: []migrate.Operation{
				&migrate.CreateTable{
					Name: "products",
					Fields: []migrate.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true},
						{Name: "name", Type: "varchar", Length: 100},
					},
					Indexes: []migrate.Index{
						{Name: "idx_products_name", Fields: []string{"name"}, Unique: false},
					},
				},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_with_idx"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "idx_products_name") {
		t.Error("expected index name in output")
	}
}

func TestSquashGenerator_GenerateSquash_FieldWithForeignKey(t *testing.T) {
	migrations := []*migrate.Migration{
		{
			Name: "0001_fk",
			Operations: []migrate.Operation{
				&migrate.AddField{
					Table: "orders",
					Field: migrate.Field{
						Name: "user_id",
						Type: "foreign_key",
						ForeignKey: &migrate.ForeignKey{
							Table:    "users",
							OnDelete: "CASCADE",
						},
					},
				},
			},
		},
	}
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{"0001_fk"}, migrations)
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "ForeignKey") {
		t.Error("expected ForeignKey in output")
	}
	if !strings.Contains(src, "CASCADE") {
		t.Error("expected CASCADE in output")
	}
}

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

func TestSquashGenerator_GenerateSquash_HasPackageMain(t *testing.T) {
	g := codegen.NewSquashGenerator()
	src, err := g.GenerateSquash("0001_squash", []string{}, []*migrate.Migration{})
	if err != nil {
		t.Fatalf("GenerateSquash: %v", err)
	}
	if !strings.Contains(src, "package main") {
		t.Error("expected 'package main' in squash output")
	}
	if !strings.Contains(src, "func init()") {
		t.Error("expected 'func init()' in squash output")
	}
}
