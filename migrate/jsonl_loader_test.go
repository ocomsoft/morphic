package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTemp creates a temporary JSONL file with the given content and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	return path
}

// TestLoadJSONLRows_ValidFile verifies that a well-formed JSONL file with mixed
// types (string, int, float, bool, null) is parsed into the correct Go types.
func TestLoadJSONLRows_ValidFile(t *testing.T) {
	content := `{"name": "alice", "age": 30, "score": 9.5, "active": true, "notes": null}
{"name": "bob", "age": 25, "score": 8.0, "active": false, "notes": "good"}
`
	path := writeTemp(t, content)

	rows, err := loadJSONLRows(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// First row checks.
	row := rows[0]

	if name, ok := row["name"].(string); !ok || name != "alice" {
		t.Errorf("row[0].name: expected \"alice\", got %v (%T)", row["name"], row["name"])
	}

	if age, ok := row["age"].(int64); !ok || age != 30 {
		t.Errorf("row[0].age: expected int64(30), got %v (%T)", row["age"], row["age"])
	}

	if score, ok := row["score"].(float64); !ok || score != 9.5 {
		t.Errorf("row[0].score: expected float64(9.5), got %v (%T)", row["score"], row["score"])
	}

	if active, ok := row["active"].(bool); !ok || !active {
		t.Errorf("row[0].active: expected true, got %v (%T)", row["active"], row["active"])
	}

	if row["notes"] != nil {
		t.Errorf("row[0].notes: expected nil, got %v (%T)", row["notes"], row["notes"])
	}

	// Second row: verify bool false and string notes.
	row2 := rows[1]

	if active, ok := row2["active"].(bool); !ok || active {
		t.Errorf("row[1].active: expected false, got %v (%T)", row2["active"], row2["active"])
	}

	if notes, ok := row2["notes"].(string); !ok || notes != "good" {
		t.Errorf("row[1].notes: expected \"good\", got %v (%T)", row2["notes"], row2["notes"])
	}
}

// TestLoadJSONLRows_EmptyFile verifies that an empty file returns an error.
func TestLoadJSONLRows_EmptyFile(t *testing.T) {
	path := writeTemp(t, "")

	_, err := loadJSONLRows(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}

	if got := err.Error(); !contains(got, "no valid rows") {
		t.Errorf("expected 'no valid rows' in error, got: %s", got)
	}
}

// TestLoadJSONLRows_MalformedLine verifies that invalid JSON on a specific line
// produces an error mentioning the line number.
func TestLoadJSONLRows_MalformedLine(t *testing.T) {
	content := `{"id": 1}
{this is not valid json}
{"id": 3}
`
	path := writeTemp(t, content)

	_, err := loadJSONLRows(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}

	msg := err.Error()
	if !contains(msg, "line 2") {
		t.Errorf("expected error to mention 'line 2', got: %s", msg)
	}
}

// TestLoadJSONLRows_SkipsEmptyLines verifies that blank lines (including
// whitespace-only lines) are ignored and do not affect the result.
func TestLoadJSONLRows_SkipsEmptyLines(t *testing.T) {
	content := `
{"id": 1}


{"id": 2}

`
	path := writeTemp(t, content)

	rows, err := loadJSONLRows(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if id, ok := rows[0]["id"].(int64); !ok || id != 1 {
		t.Errorf("row[0].id: expected int64(1), got %v (%T)", rows[0]["id"], rows[0]["id"])
	}

	if id, ok := rows[1]["id"].(int64); !ok || id != 2 {
		t.Errorf("row[1].id: expected int64(2), got %v (%T)", rows[1]["id"], rows[1]["id"])
	}
}

// TestLoadJSONLRows_SkipsCommentLines verifies that lines starting with # are
// ignored, allowing comments in JSONL files.
func TestLoadJSONLRows_SkipsCommentLines(t *testing.T) {
	content := `# Countries seed data
{"code": "AU", "name": "Australia"}
# New Zealand added 2026-06-26
{"code": "NZ", "name": "New Zealand"}
`
	path := writeTemp(t, content)

	rows, err := loadJSONLRows(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0]["code"] != "AU" {
		t.Errorf("row[0].code: expected AU, got %v", rows[0]["code"])
	}

	if rows[1]["code"] != "NZ" {
		t.Errorf("row[1].code: expected NZ, got %v", rows[1]["code"])
	}
}

// TestLoadJSONLRows_FileNotFound verifies that a non-existent file returns an error.
func TestLoadJSONLRows_FileNotFound(t *testing.T) {
	_, err := loadJSONLRows("/nonexistent/path/to/file.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadJSONLRows_NumericTypes verifies that integers are returned as int64
// and floating-point numbers as float64, preserving numeric precision.
func TestLoadJSONLRows_NumericTypes(t *testing.T) {
	content := `{"int_val": 42, "neg_int": -7, "float_val": 3.14, "big_int": 9999999999999, "zero": 0, "float_zero": 0.0}
`
	path := writeTemp(t, content)

	rows, err := loadJSONLRows(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]

	tests := []struct {
		key      string
		wantType string
	}{
		{"int_val", "int64"},
		{"neg_int", "int64"},
		{"float_val", "float64"},
		{"big_int", "int64"},
		{"zero", "int64"},
		{"float_zero", "float64"},
	}

	for _, tc := range tests {
		val := row[tc.key]

		var gotType string

		switch val.(type) {
		case int64:
			gotType = "int64"
		case float64:
			gotType = "float64"
		default:
			gotType = "unknown"
		}

		if gotType != tc.wantType {
			t.Errorf("%s: expected type %s, got %s (value: %v)", tc.key, tc.wantType, gotType, val)
		}
	}

	// Verify specific values.
	if v := row["int_val"].(int64); v != 42 {
		t.Errorf("int_val: expected 42, got %d", v)
	}

	if v := row["neg_int"].(int64); v != -7 {
		t.Errorf("neg_int: expected -7, got %d", v)
	}

	if v := row["float_val"].(float64); v != 3.14 {
		t.Errorf("float_val: expected 3.14, got %f", v)
	}

	if v := row["big_int"].(int64); v != 9999999999999 {
		t.Errorf("big_int: expected 9999999999999, got %d", v)
	}
}

// contains checks if substr appears in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

// searchString returns true if substr is found anywhere in s.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
