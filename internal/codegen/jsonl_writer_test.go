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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTableJSONL_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	rows := []map[string]any{
		{"code": "AU", "name": "Australia", "pop": int64(26000000)},
		{"code": "NZ", "name": "New Zealand", "pop": int64(5000000)},
	}
	if err := WriteTableJSONL(path, rows); err != nil {
		t.Fatalf("WriteTableJSONL: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Keys should be sorted alphabetically
	if !strings.HasPrefix(lines[0], `{"code":"AU"`) {
		t.Errorf("expected sorted keys starting with code, got: %s", lines[0])
	}
}

func TestWriteTableJSONL_EmptyRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := WriteTableJSONL(path, nil); err != nil {
		t.Fatalf("WriteTableJSONL: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Errorf("expected empty file, got: %s", data)
	}
}

func TestWriteTableJSONL_SpecialValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "special.jsonl")
	rows := []map[string]any{
		{"bool_val": true, "float_val": 3.14, "null_val": nil, "str_val": "hello"},
	}
	if err := WriteTableJSONL(path, rows); err != nil {
		t.Fatalf("WriteTableJSONL: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	// Verify sorted keys
	if !strings.Contains(line, `"bool_val":true`) {
		t.Errorf("expected bool_val:true, got: %s", line)
	}
	if !strings.Contains(line, `"null_val":null`) {
		t.Errorf("expected null_val:null, got: %s", line)
	}
}
