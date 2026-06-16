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

// Package interp loads morphic migration .go files into an in-process
// *migrate.Registry using the yaegi interpreter, removing the need to compile
// the migrations module with the Go toolchain.
package interp

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"

	"github.com/ocomsoft/morphic/migrate"
	"github.com/ocomsoft/morphic/migrate/symbols"
)

// virtualPkg is the package name used inside the in-memory filesystem when
// rewriting migration source files. The on-disk files declare `package main`
// (so that `go build ./migrations` still works), but yaegi cannot import a
// package named `main`, so we rename it before evaluation.
const virtualPkg = "migrations"

// LoadRegistry auto-detects the migration format in migrationsDir and loads
// all migrations into a *migrate.Registry. If both .go and .star files exist,
// it returns an error (mixed formats are not supported).
func LoadRegistry(migrationsDir string) (*migrate.Registry, error) {
	goFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	starFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.star"))

	// Filter out main.go and test files from Go file count.
	var goMigFiles []string
	for _, f := range goFiles {
		base := filepath.Base(f)
		if base != "main.go" && !strings.HasSuffix(base, "_test.go") {
			goMigFiles = append(goMigFiles, f)
		}
	}

	hasGo := len(goMigFiles) > 0
	hasStar := len(starFiles) > 0

	if hasGo && hasStar {
		return nil, fmt.Errorf("mixed migration formats: found both .go and .star files in %s", migrationsDir)
	}

	if hasStar {
		return LoadStarlarkRegistry(migrationsDir)
	}
	return LoadGoRegistry(migrationsDir)
}

// LoadGoRegistry reads every *.go file in migrationsDir (except main.go),
// interprets them with yaegi, and returns a freshly-populated *migrate.Registry.
//
// Each migration file's init() calls migrate.Register, which is intercepted
// here and routed into the returned registry rather than the package-level
// global. This lets the function be called multiple times in a single process
// without duplicate-registration panics.
func LoadGoRegistry(migrationsDir string) (*migrate.Registry, error) {
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("scanning migrations directory: %w", err)
	}

	// Filter out main.go and *_test.go; sort for deterministic eval order.
	var migFiles []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == "main.go" {
			continue
		}
		if len(base) > 8 && base[len(base)-8:] == "_test.go" {
			continue
		}
		migFiles = append(migFiles, f)
	}
	sort.Strings(migFiles)

	if len(migFiles) == 0 {
		return migrate.NewRegistry(), nil
	}

	// Build an in-memory filesystem with each file rewritten to declare
	// `package migrations`. yaegi treats the directory as a single multi-file
	// package, dedupes imports, and runs all init() funcs in source order.
	fsys := fstest.MapFS{}
	for _, path := range migFiles {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", path, readErr)
		}
		rewritten, rewriteErr := rewritePackage(path, data, virtualPkg)
		if rewriteErr != nil {
			return nil, fmt.Errorf("rewriting %s: %w", path, rewriteErr)
		}
		fsys["src/"+virtualPkg+"/"+filepath.Base(path)] = &fstest.MapFile{Data: rewritten}
	}

	reg := migrate.NewRegistry()

	i := interp.New(interp.Options{SourcecodeFilesystem: fsys})
	if err := i.Use(stdlib.Symbols); err != nil {
		return nil, fmt.Errorf("registering stdlib symbols: %w", err)
	}
	if err := i.Use(perLoadSymbols(reg)); err != nil {
		return nil, fmt.Errorf("registering migrate symbols: %w", err)
	}

	if _, err := i.Eval(`import _ "` + virtualPkg + `"`); err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	return reg, nil
}

// perLoadSymbols clones symbols.Symbols and overrides the migrate package's
// Register entry to write into reg instead of the global registry. This
// isolation lets multiple LoadRegistry calls coexist in one process.
func perLoadSymbols(reg *migrate.Registry) map[string]map[string]reflect.Value {
	out := make(map[string]map[string]reflect.Value, len(symbols.Symbols))
	for pkg, syms := range symbols.Symbols {
		copied := make(map[string]reflect.Value, len(syms))
		for name, val := range syms {
			copied[name] = val
		}
		out[pkg] = copied
	}
	const migPkg = "github.com/ocomsoft/morphic/migrate/migrate"
	if out[migPkg] == nil {
		out[migPkg] = map[string]reflect.Value{}
	}
	out[migPkg]["Register"] = reflect.ValueOf(func(m *migrate.Migration) {
		reg.Register(m)
	})
	return out
}

const (
	legacyImportPrefix  = "github.com/ocomsoft/makemigrations/"
	currentImportPrefix = "github.com/ocomsoft/morphic/"
)

// rewritePackage parses src, replaces the package name with newName, and
// silently rewrites legacy makemigrations import paths to morphic.
// Comments and formatting are preserved.
func rewritePackage(filename string, src []byte, newName string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	file.Name = ast.NewIdent(newName)
	rewriteLegacyImports(file)
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// rewriteLegacyImports rewrites any import paths starting with the old
// makemigrations module path to the current morphic path, so pre-rebrand
// migration files load without manual edits.
func rewriteLegacyImports(file *ast.File) {
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.HasPrefix(path, legacyImportPrefix) {
			imp.Path.Value = `"` + currentImportPrefix + strings.TrimPrefix(path, legacyImportPrefix) + `"`
		}
	}
}
