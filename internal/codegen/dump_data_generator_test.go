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
)

func TestDumpDataGenerator_SingleTable(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "unit_type",
			ConflictKeys: []string{"id"},
			Rows: []map[string]any{
				{
					"id":          "9163b64b-cdda-4cb8-9e28-12afc8581e36",
					"code":        "I",
					"description": "Imperial",
				},
				{
					"id":          "abc123",
					"code":        "M",
					"description": nil,
				},
			},
		},
	}

	src, err := g.Generate("0003_dump_unit_type", []string{"0002_update_schema"}, tables)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify the output compiles as valid Go via format.Source.
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("generated source does not compile: %v\nSource:\n%s", err, src)
	}

	if !strings.Contains(src, `"unit_type"`) {
		t.Error("expected table name in output")
	}
	if !strings.Contains(src, "UpsertData") {
		t.Error("expected UpsertData operation in output")
	}
	if !strings.Contains(src, `"id"`) {
		t.Error("expected conflict key in output")
	}
	if !strings.Contains(src, `"Imperial"`) {
		t.Error("expected row value in output")
	}
	if !strings.Contains(src, "nil") {
		t.Error("expected nil value in output for nil field")
	}
	// Verify column keys are sorted alphabetically: code < description < id.
	codeIdx := strings.Index(src, `"code"`)
	descIdx := strings.Index(src, `"description"`)
	idIdx := strings.Index(src, `"id"`)
	if codeIdx == -1 || descIdx == -1 || idIdx == -1 {
		t.Fatal("expected all column keys to be present")
	}
}

func TestDumpDataGenerator_MultiTable(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "countries",
			ConflictKeys: []string{"code"},
			Rows: []map[string]any{
				{"code": "AU", "name": "Australia"},
			},
		},
		{
			Table:        "states",
			ConflictKeys: []string{"id"},
			Rows: []map[string]any{
				{"id": int64(1), "name": "Queensland", "country_code": "AU"},
			},
		},
	}

	src, err := g.Generate("0004_dump_geo", []string{"0003_prev"}, tables)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(src, `"countries"`) {
		t.Error("expected first table name in output")
	}
	if !strings.Contains(src, `"states"`) {
		t.Error("expected second table name in output")
	}
}

func TestDumpDataGenerator_EmptyTables(t *testing.T) {
	g := codegen.NewDumpDataGenerator()

	_, err := g.Generate("0003_dump", []string{"0002_prev"}, nil)
	if err == nil {
		t.Fatal("expected error for nil tables, got nil")
	}

	_, err = g.Generate("0003_dump", []string{"0002_prev"}, []codegen.TableDump{})
	if err == nil {
		t.Fatal("expected error for empty tables, got nil")
	}
}

func TestDumpDataGenerator_Starlark_SingleTable(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "unit_type",
			ConflictKeys: []string{"id"},
			Rows: []map[string]any{
				{
					"id":          "9163b64b-cdda-4cb8-9e28-12afc8581e36",
					"code":        "I",
					"description": "Imperial",
				},
				{
					"id":          "abc123",
					"code":        "M",
					"description": nil,
				},
			},
		},
	}

	src, err := g.GenerateStarlark("0003_dump_unit_type", []string{"0002_update_schema"}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlark: %v", err)
	}

	if !strings.Contains(src, "migration(") {
		t.Error("expected migration( in output")
	}
	if !strings.Contains(src, `name = "0003_dump_unit_type"`) {
		t.Error("expected migration name in output")
	}
	if !strings.Contains(src, `"0002_update_schema"`) {
		t.Error("expected dependency in output")
	}
	if !strings.Contains(src, `upsert_data("unit_type"`) {
		t.Error("expected upsert_data call in output")
	}
	if !strings.Contains(src, `conflict_keys = ["id"]`) {
		t.Error("expected conflict_keys in output")
	}
	if !strings.Contains(src, "row(") {
		t.Error("expected row() call in output")
	}
	if !strings.Contains(src, `description="Imperial"`) {
		t.Error("expected row value in output")
	}
	if !strings.Contains(src, "description=None") {
		t.Error("expected None for nil value in output")
	}
	// Must NOT contain Go-specific syntax
	if strings.Contains(src, "package main") {
		t.Error("Starlark output must not contain 'package main'")
	}
	if strings.Contains(src, "func init()") {
		t.Error("Starlark output must not contain 'func init()'")
	}
}

func TestDumpDataGenerator_Starlark_MultiTable(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "countries",
			ConflictKeys: []string{"code"},
			Rows: []map[string]any{
				{"code": "AU", "name": "Australia"},
			},
		},
		{
			Table:        "states",
			ConflictKeys: []string{"id"},
			Rows: []map[string]any{
				{"id": int64(1), "name": "Queensland", "country_code": "AU"},
			},
		},
	}

	src, err := g.GenerateStarlark("0004_dump_geo", []string{"0003_prev"}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlark: %v", err)
	}

	if !strings.Contains(src, `upsert_data("countries"`) {
		t.Error("expected first table in output")
	}
	if !strings.Contains(src, `upsert_data("states"`) {
		t.Error("expected second table in output")
	}
}

func TestDumpDataGenerator_Starlark_EmptyTables(t *testing.T) {
	g := codegen.NewDumpDataGenerator()

	_, err := g.GenerateStarlark("0003_dump", []string{"0002_prev"}, nil)
	if err == nil {
		t.Fatal("expected error for nil tables, got nil")
	}

	_, err = g.GenerateStarlark("0003_dump", []string{"0002_prev"}, []codegen.TableDump{})
	if err == nil {
		t.Fatal("expected error for empty tables, got nil")
	}
}

func TestDumpDataGenerator_Starlark_BoolAndNumericValues(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "settings",
			ConflictKeys: []string{"key"},
			Rows: []map[string]any{
				{"key": "enabled", "bool_val": true, "int_val": int64(42), "float_val": 3.14},
			},
		},
	}

	src, err := g.GenerateStarlark("0001_dump_settings", []string{}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlark: %v", err)
	}

	if !strings.Contains(src, "bool_val=True") {
		t.Error("expected True for bool value")
	}
	if !strings.Contains(src, "int_val=42") {
		t.Error("expected integer value")
	}
	if !strings.Contains(src, "float_val=3.14") {
		t.Error("expected float value")
	}
}

func TestDumpDataGenerator_NoDeps(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "config",
			ConflictKeys: []string{"key"},
			Rows: []map[string]any{
				{"key": "version", "value": "1.0"},
			},
		},
	}

	src, err := g.Generate("0001_dump_config", []string{}, tables)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(src, `Dependencies: []string{}`) {
		t.Errorf("expected empty dependencies slice, got:\n%s", src)
	}
}
