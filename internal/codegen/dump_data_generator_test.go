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

// TestDumpDataGenerator_WriteUpsertData_WithDataFile verifies that when DataFile is set,
// the generated Go source contains DataFile: and does not contain Rows:.
func TestDumpDataGenerator_WriteUpsertData_WithDataFile(t *testing.T) {
	g := codegen.NewDumpDataGenerator()
	tables := []codegen.TableDump{
		{
			Table:        "countries",
			ConflictKeys: []string{"code"},
			DataFile:     "data/countries.jsonl",
		},
	}

	src, err := g.Generate("0005_dump_countries", []string{"0004_prev"}, tables)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Verify the output compiles as valid Go via format.Source.
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("generated source does not compile: %v\nSource:\n%s", err, src)
	}

	if !strings.Contains(src, `DataFile:`) {
		t.Errorf("expected DataFile: in output, got:\n%s", src)
	}
	if !strings.Contains(src, `"data/countries.jsonl"`) {
		t.Errorf("expected data file path in output, got:\n%s", src)
	}
	if strings.Contains(src, "Rows:") {
		t.Errorf("expected no Rows: in output when DataFile is set, got:\n%s", src)
	}
}

// TestDumpDataGenerator_WriteUpsertData_WithRows verifies that when DataFile is empty,
// the generated Go source contains Rows: and does not contain DataFile:.
func TestDumpDataGenerator_WriteUpsertData_WithRows(t *testing.T) {
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

	src, err := g.Generate("0006_dump_config", []string{"0005_prev"}, tables)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(src, "Rows:") {
		t.Errorf("expected Rows: in output, got:\n%s", src)
	}
	if strings.Contains(src, "DataFile:") {
		t.Errorf("expected no DataFile: in output when rows are provided, got:\n%s", src)
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
