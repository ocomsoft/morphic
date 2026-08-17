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
package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestCountMigrationFiles_StarOnly verifies that countMigrationFiles discovers
// .star migration files when the directory contains only Starlark migrations.
func TestCountMigrationFiles_StarOnly(t *testing.T) {
	dir := t.TempDir()

	starFiles := []string{
		"0001_initial.star",
		"0002_add_users.star",
		"0003_add_indexes.star",
	}
	for _, name := range starFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# starlark migration"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := countMigrationFiles(dir)
	if err != nil {
		t.Fatalf("countMigrationFiles returned error: %v", err)
	}
	if len(got) != len(starFiles) {
		t.Fatalf("expected %d migration files, got %d", len(starFiles), len(got))
	}

	var gotNames []string
	for _, f := range got {
		gotNames = append(gotNames, filepath.Base(f))
	}
	sort.Strings(gotNames)
	for i, want := range starFiles {
		if gotNames[i] != want {
			t.Errorf("file[%d] = %q, want %q", i, gotNames[i], want)
		}
	}
}

// TestCountMigrationFiles_GoOnly verifies that .go migrations (excluding
// main.go and test files) are still discovered correctly.
func TestCountMigrationFiles_GoOnly(t *testing.T) {
	dir := t.TempDir()

	files := map[string]bool{
		"0001_initial.go":      true,
		"0002_add_users.go":    true,
		"main.go":              false,
		"0001_initial_test.go": false,
	}
	for name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package migrations"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := countMigrationFiles(dir)
	if err != nil {
		t.Fatalf("countMigrationFiles returned error: %v", err)
	}

	wantCount := 0
	for _, include := range files {
		if include {
			wantCount++
		}
	}
	if len(got) != wantCount {
		t.Fatalf("expected %d migration files, got %d: %v", wantCount, len(got), got)
	}

	for _, f := range got {
		base := filepath.Base(f)
		if base == "main.go" {
			t.Error("main.go should be excluded")
		}
		if filepath.Ext(base) == ".go" && len(base) > 8 && base[len(base)-8:] == "_test.go" {
			t.Errorf("test file %s should be excluded", base)
		}
	}
}

// TestCountMigrationFiles_Mixed verifies that both .go and .star files are
// discovered and returned in sorted order.
func TestCountMigrationFiles_Mixed(t *testing.T) {
	dir := t.TempDir()

	allFiles := []string{
		"0001_initial.go",
		"0002_add_users.star",
		"0003_indexes.go",
		"main.go",
	}
	for _, name := range allFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := countMigrationFiles(dir)
	if err != nil {
		t.Fatalf("countMigrationFiles returned error: %v", err)
	}

	want := []string{"0001_initial.go", "0002_add_users.star", "0003_indexes.go"}
	if len(got) != len(want) {
		t.Fatalf("expected %d files, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if filepath.Base(got[i]) != w {
			t.Errorf("file[%d] = %q, want %q", i, filepath.Base(got[i]), w)
		}
	}
}

// TestCountMigrationFiles_EmptyDir returns empty slice for an empty directory.
func TestCountMigrationFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	got, err := countMigrationFiles(dir)
	if err != nil {
		t.Fatalf("countMigrationFiles returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 migration files, got %d", len(got))
	}
}
