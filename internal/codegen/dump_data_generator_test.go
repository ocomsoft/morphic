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
)

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

func TestGenerateStarlarkWithRowsFile_SingleTable(t *testing.T) {
	gen := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "countries",
			ConflictKeys: []string{"code"},
			Rows:         []map[string]any{{"code": "AU"}},
		},
	}
	src, err := gen.GenerateStarlarkWithRowsFile("0003_dump_countries", []string{"0002_initial"}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlarkWithRowsFile: %v", err)
	}
	if !strings.Contains(src, `rows_file = "0003_dump_countries_countries.jsonl"`) {
		t.Errorf("expected rows_file reference, got:\n%s", src)
	}
	if strings.Contains(src, "rows = [") {
		t.Errorf("should not contain inline rows, got:\n%s", src)
	}
}

func TestGenerateStarlarkWithRowsFile_MultiTable(t *testing.T) {
	gen := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{Table: "countries", ConflictKeys: []string{"code"}, Rows: []map[string]any{{"code": "AU"}}},
		{Table: "currencies", ConflictKeys: []string{"code"}, Rows: []map[string]any{{"code": "AUD"}}},
	}
	src, err := gen.GenerateStarlarkWithRowsFile("0003_dump_data", []string{"0002_initial"}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlarkWithRowsFile: %v", err)
	}
	if !strings.Contains(src, `rows_file = "0003_dump_data_countries.jsonl"`) {
		t.Errorf("expected countries rows_file, got:\n%s", src)
	}
	if !strings.Contains(src, `rows_file = "0003_dump_data_currencies.jsonl"`) {
		t.Errorf("expected currencies rows_file, got:\n%s", src)
	}
}

func TestGenerateStarlarkWithRowsFile_EmptyTables(t *testing.T) {
	gen := codegen.NewDumpDataGenerator()

	_, err := gen.GenerateStarlarkWithRowsFile("0003_dump", []string{"0002_prev"}, nil)
	if err == nil {
		t.Fatal("expected error for nil tables, got nil")
	}

	_, err = gen.GenerateStarlarkWithRowsFile("0003_dump", []string{"0002_prev"}, []codegen.TableDump{})
	if err == nil {
		t.Fatal("expected error for empty tables, got nil")
	}
}

func TestDumpDataGenerator_Starlark_NoDeps(t *testing.T) {
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

	src, err := g.GenerateStarlark("0001_dump_config", []string{}, tables)
	if err != nil {
		t.Fatalf("GenerateStarlark: %v", err)
	}

	if !strings.Contains(src, `dependencies = []`) {
		t.Errorf("expected empty dependencies list, got:\n%s", src)
	}
}
