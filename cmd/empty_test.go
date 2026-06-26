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

package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
)

// TestRunEmpty_WritesBlankFile verifies that the blank generator creates a file
// with an empty operations block and a TODO comment.
func TestRunEmpty_WritesBlankFile(t *testing.T) {
	tmpDir := t.TempDir()

	gen := codegen.NewStarlarkGenerator()
	src, err := gen.GenerateBlank("0001_blank", []string{})
	if err != nil {
		t.Fatalf("GenerateBlank: %v", err)
	}

	outPath := filepath.Join(tmpDir, "0001_blank.star")
	if err := os.WriteFile(outPath, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(got)
	if !strings.Contains(content, "0001_blank") {
		t.Error("expected migration name in file")
	}
	if !strings.Contains(content, "TODO") {
		t.Error("expected TODO comment in blank migration file")
	}
	if !strings.Contains(content, "operations") {
		t.Error("expected operations field in file")
	}
}

// TestRunEmpty_NoDepsWhenNoMigrations verifies that a blank migration generated
// with no prior migrations has an empty dependencies list.
func TestRunEmpty_NoDepsWhenNoMigrations(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	src, err := gen.GenerateBlank("0001_blank", []string{})
	if err != nil {
		t.Fatalf("GenerateBlank: %v", err)
	}

	if !strings.Contains(src, `dependencies = []`) {
		t.Errorf("expected empty dependencies, got:\n%s", src)
	}
}

// TestRunEmpty_WithDeps verifies that dependencies are included correctly.
func TestRunEmpty_WithDeps(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	src, err := gen.GenerateBlank("0003_blank", []string{"0002_add_users"})
	if err != nil {
		t.Fatalf("GenerateBlank: %v", err)
	}

	if !strings.Contains(src, `"0002_add_users"`) {
		t.Errorf("expected dependency in generated source, got:\n%s", src)
	}
}

// TestRunEmpty_CustomName verifies that the migration name suffix is used.
func TestRunEmpty_CustomName(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	src, err := gen.GenerateBlank("0004_add_triggers", []string{})
	if err != nil {
		t.Fatalf("GenerateBlank: %v", err)
	}

	if !strings.Contains(src, `"0004_add_triggers"`) {
		t.Errorf("expected custom name in generated source, got:\n%s", src)
	}
}
