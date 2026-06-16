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

// schema_loader.go auto-detects whether a schema directory contains a .star or
// .yaml schema file and loads it into a *types.Schema.
package interp

import (
	"fmt"
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/ocomsoft/morphic/internal/types"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

// LoadSchema auto-detects the schema format in schemaDir and returns a
// *types.Schema. It looks for schema.star and schema.yaml. If both exist
// it returns an error; if neither exists it also returns an error.
func LoadSchema(schemaDir string, verbose bool) (*types.Schema, error) {
	starPath := filepath.Join(schemaDir, "schema.star")
	yamlPath := filepath.Join(schemaDir, "schema.yaml")

	hasStar := fileExists(starPath)
	hasYAML := fileExists(yamlPath)

	switch {
	case hasStar && hasYAML:
		return nil, fmt.Errorf("found both schema.star and schema.yaml in %s; use only one format", schemaDir)
	case !hasStar && !hasYAML:
		return nil, fmt.Errorf("no schema file found in %s (expected schema.star or schema.yaml)", schemaDir)
	case hasStar:
		return loadStarlarkSchema(starPath, verbose)
	default:
		return loadYAMLSchema(yamlPath, verbose)
	}
}

// loadStarlarkSchema reads and executes a .star schema file, returning the
// collected types.Schema.
func loadStarlarkSchema(path string, verbose bool) (*types.Schema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	builtins := NewSchemaDSLBuiltins()
	thread := &starlark.Thread{Name: filepath.Base(path)}

	_, err = starlark.ExecFileOptions(&syntax.FileOptions{}, thread, filepath.Base(path), data, builtins.Env())
	if err != nil {
		return nil, fmt.Errorf("evaluating %s: %w", path, err)
	}

	schema := builtins.Collected()
	if verbose {
		fmt.Printf("Loaded Starlark schema: %s v%s with %d tables\n",
			schema.Database.Name, schema.Database.Version, len(schema.Tables))
	}
	return schema, nil
}

// loadYAMLSchema delegates to the existing YAML parser.
func loadYAMLSchema(path string, verbose bool) (*types.Schema, error) {
	parser := yamlpkg.NewParser(verbose)
	return parser.ParseSchemaFile(path)
}

// fileExists reports whether the named file exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
