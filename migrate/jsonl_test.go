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

	"github.com/ocomsoft/morphic/migrate"
)

// TestReadJSONL_HappyPath writes a temp JSONL file, reads it back, and
// verifies the returned rows match.
func TestReadJSONL_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")

	content := `{"code":"AU","name":"Australia"}
{"code":"US","name":"United States"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	rows, err := migrate.ReadJSONL(path)
	if err != nil {
		t.Fatalf("ReadJSONL error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["code"] != "AU" {
		t.Errorf("row 0 code: got %v, want AU", rows[0]["code"])
	}
	if rows[1]["name"] != "United States" {
		t.Errorf("row 1 name: got %v, want United States", rows[1]["name"])
	}
}

// TestReadJSONL_SkipsEmptyLines verifies that blank lines are ignored.
func TestReadJSONL_SkipsEmptyLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")

	content := `{"code":"AU"}

{"code":"US"}

`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	rows, err := migrate.ReadJSONL(path)
	if err != nil {
		t.Fatalf("ReadJSONL error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// TestReadJSONL_SkipsComments verifies that lines starting with "//" are skipped.
func TestReadJSONL_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")

	content := `// This is a comment
{"code":"AU"}
// Another comment
{"code":"US"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	rows, err := migrate.ReadJSONL(path)
	if err != nil {
		t.Fatalf("ReadJSONL error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// TestReadJSONL_ParseError verifies that a malformed JSON line returns an error
// with the correct line number.
func TestReadJSONL_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")

	content := `{"code":"AU"}
not valid json
{"code":"US"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := migrate.ReadJSONL(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("expected error to mention line 2, got: %v", err)
	}
}

// TestReadJSONL_FileNotFound verifies that a nonexistent path returns an error.
func TestReadJSONL_FileNotFound(t *testing.T) {
	_, err := migrate.ReadJSONL("/nonexistent/path/data.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

// TestWriteJSONL_HappyPath writes rows and reads them back to verify round-trip.
func TestWriteJSONL_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.jsonl")

	rows := []map[string]any{
		{"code": "AU", "name": "Australia"},
		{"code": "US", "name": "United States"},
	}

	if err := migrate.WriteJSONL(path, rows); err != nil {
		t.Fatalf("WriteJSONL error: %v", err)
	}

	got, err := migrate.ReadJSONL(path)
	if err != nil {
		t.Fatalf("ReadJSONL error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0]["code"] != "AU" {
		t.Errorf("row 0 code: got %v, want AU", got[0]["code"])
	}
	if got[1]["name"] != "United States" {
		t.Errorf("row 1 name: got %v, want United States", got[1]["name"])
	}
}

// TestWriteJSONL_CreatesParentDirs verifies that WriteJSONL creates parent
// directories that do not yet exist.
func TestWriteJSONL_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "out.jsonl")

	rows := []map[string]any{{"key": "value"}}
	if err := migrate.WriteJSONL(path, rows); err != nil {
		t.Fatalf("WriteJSONL error: %v", err)
	}

	// Verify the file exists and is readable.
	got, err := migrate.ReadJSONL(path)
	if err != nil {
		t.Fatalf("ReadJSONL error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
}
