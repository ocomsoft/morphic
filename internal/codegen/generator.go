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

import "github.com/ocomsoft/morphic/internal/yaml"

// MigrationFormat identifies the output language for generated migration files.
type MigrationFormat string

const (
	FormatStarlark MigrationFormat = "starlark"
)

// MigrationGenerator generates migration source code in a specific language.
type MigrationGenerator interface {
	// GenerateMigration generates a migration file from a schema diff.
	GenerateMigration(
		name string,
		deps []string,
		diff *yaml.SchemaDiff,
		currentSchema, previousSchema *yaml.Schema,
		decisions map[int]yaml.PromptResponse,
	) (string, error)

	// GenerateBlank generates an empty migration with a TODO placeholder.
	GenerateBlank(name string, deps []string) (string, error)

	// FileExtension returns the file extension for this format (e.g. ".go", ".star").
	FileExtension() string
}

// MigrationFileNameForFormat returns the .star file name for a migration.
func MigrationFileNameForFormat(name string, _ MigrationFormat) string {
	return name + ".star"
}

// FormatFromExtension returns the migration format (always Starlark).
func FormatFromExtension(_ string) MigrationFormat {
	return FormatStarlark
}

// ParseMigrationFormat parses a format string from config (always returns Starlark).
func ParseMigrationFormat(_ string) MigrationFormat {
	return FormatStarlark
}
