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
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.starlark.net/starlark"

	"github.com/ocomsoft/morphic/migrate"
)

// LoadStarlarkRegistry reads every *.star file in migrationsDir, evaluates them
// with the migration DSL builtins, and returns a populated *migrate.Registry.
func LoadStarlarkRegistry(migrationsDir string) (*migrate.Registry, error) {
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.star"))
	if err != nil {
		return nil, fmt.Errorf("scanning starlark migrations: %w", err)
	}
	sort.Strings(files)

	reg := migrate.NewRegistry()

	if len(files) == 0 {
		return reg, nil
	}

	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", path, readErr)
		}

		builtins := NewStarlarkBuiltins()
		thread := &starlark.Thread{Name: filepath.Base(path)}

		_, execErr := starlark.ExecFile(thread, filepath.Base(path), data, builtins.Env())
		if execErr != nil {
			return nil, fmt.Errorf("evaluating %s: %w", path, execErr)
		}

		m := builtins.Collected()
		if m == nil {
			return nil, fmt.Errorf("%s: no migration() call found", path)
		}
		reg.Register(m)
	}
	return reg, nil
}
