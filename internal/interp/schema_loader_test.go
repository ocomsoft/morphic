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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSchema_Starlark(t *testing.T) {
	dir := t.TempDir()
	src := `
database("myapp", "2.0.0")
table("accounts",
    fields = [
        serial("id", primary_key=True),
        varchar("email", 255),
    ],
)
`
	if err := os.WriteFile(filepath.Join(dir, "schema.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema, err := LoadSchema(dir, false)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if schema.Database.Name != "myapp" {
		t.Errorf("db name = %q, want %q", schema.Database.Name, "myapp")
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(schema.Tables))
	}
	if schema.Tables[0].Name != "accounts" {
		t.Errorf("table name = %q, want %q", schema.Tables[0].Name, "accounts")
	}
}

func TestLoadSchema_YAML(t *testing.T) {
	dir := t.TempDir()
	src := `
database:
  name: myapp
  version: "1.0.0"
tables:
  - name: users
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: name
        type: varchar
        length: 100
`
	if err := os.WriteFile(filepath.Join(dir, "schema.yaml"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	schema, err := LoadSchema(dir, false)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if schema.Database.Name != "myapp" {
		t.Errorf("db name = %q, want %q", schema.Database.Name, "myapp")
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("got %d tables, want 1", len(schema.Tables))
	}
}

func TestLoadSchema_BothExist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.star"), []byte(`database("x","1")`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.yaml"), []byte("database:\n  name: x\n  version: '1'\ntables:\n  - name: t\n    fields:\n      - name: id\n        type: integer\n        primary_key: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSchema(dir, false)
	if err == nil {
		t.Fatal("expected error when both files exist")
	}
	if !strings.Contains(err.Error(), "found both") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadSchema_NeitherExist(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadSchema(dir, false)
	if err == nil {
		t.Fatal("expected error when no schema file exists")
	}
	if !strings.Contains(err.Error(), "no schema file found") {
		t.Errorf("unexpected error: %v", err)
	}
}
