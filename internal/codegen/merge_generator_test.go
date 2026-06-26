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

func TestMergeGenerator_GenerateMerge(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateMerge("0004_merge_feature_a_and_b",
		[]string{"0003_feature_a", "0003_feature_b"})
	if err != nil {
		t.Fatalf("GenerateMerge: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v\nSource:\n%s", err, src)
	}
	if !strings.Contains(src, "0004_merge_feature_a_and_b") {
		t.Error("expected migration name in output")
	}
	if !strings.Contains(src, "0003_feature_a") {
		t.Error("expected first dependency in output")
	}
	if !strings.Contains(src, "0003_feature_b") {
		t.Error("expected second dependency in output")
	}
	// Merge migrations have no operations
	if strings.Contains(src, "CreateTable") || strings.Contains(src, "AddField") {
		t.Error("merge migration should have no operations")
	}
}

func TestMergeGenerator_GenerateMerge_EmptyDeps(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateMerge("0001_merge", []string{})
	if err != nil {
		t.Fatalf("GenerateMerge with empty deps: %v", err)
	}
	if _, err := format.Source([]byte(src)); err != nil {
		t.Fatalf("output is not valid Go: %v", err)
	}
}

func TestMergeGenerator_GenerateMerge_SingleDep(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateMerge("0002_merge", []string{"0001_initial"})
	if err != nil {
		t.Fatalf("GenerateMerge: %v", err)
	}
	if !strings.Contains(src, "0001_initial") {
		t.Error("expected dep in output")
	}
	if !strings.Contains(src, "Operations") {
		t.Error("expected Operations field in output")
	}
}

func TestMergeGenerator_GenerateStarlarkMerge(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateStarlarkMerge("0004_merge_feature_a_and_b",
		[]string{"0003_feature_a", "0003_feature_b"})
	if err != nil {
		t.Fatalf("GenerateStarlarkMerge: %v", err)
	}

	if !strings.Contains(src, "migration(") {
		t.Error("expected migration( in output")
	}
	if !strings.Contains(src, `name = "0004_merge_feature_a_and_b"`) {
		t.Error("expected migration name in output")
	}
	if !strings.Contains(src, `"0003_feature_a"`) {
		t.Error("expected first dependency in output")
	}
	if !strings.Contains(src, `"0003_feature_b"`) {
		t.Error("expected second dependency in output")
	}
	if !strings.Contains(src, "operations = []") {
		t.Error("expected empty operations list")
	}
	if strings.Contains(src, "package main") {
		t.Error("Starlark output must not contain 'package main'")
	}
}

func TestMergeGenerator_GenerateStarlarkMerge_EmptyDeps(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateStarlarkMerge("0001_merge", []string{})
	if err != nil {
		t.Fatalf("GenerateStarlarkMerge with empty deps: %v", err)
	}
	if !strings.Contains(src, "dependencies = []") {
		t.Errorf("expected empty dependencies, got:\n%s", src)
	}
}

func TestMergeGenerator_Output_HasPackageMain(t *testing.T) {
	g := codegen.NewMergeGenerator()
	src, err := g.GenerateMerge("0003_merge", []string{"0002_a", "0002_b"})
	if err != nil {
		t.Fatalf("GenerateMerge: %v", err)
	}
	if !strings.Contains(src, "package main") {
		t.Error("expected 'package main' in merge output")
	}
	if !strings.Contains(src, "func init()") {
		t.Error("expected 'func init()' in merge output")
	}
}
