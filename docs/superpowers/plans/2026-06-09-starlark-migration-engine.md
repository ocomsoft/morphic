# Starlark Migration Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Starlark as a second migration script format alongside Go/Yaegi, with file-extension-based detection (`.go` → Yaegi, `.star` → Starlark) and a config option to set the default generation format.

**Architecture:** A `MigrationGenerator` interface abstracts codegen so both `GoGenerator` and `StarlarkGenerator` implement the same contract. A `StarlarkLoader` in `internal/interp/` reads `.star` files and evaluates them via `google/starlark-go`, populating the same `*migrate.Registry` that the Yaegi loader produces. The `LoadRegistry` entry point auto-detects format by scanning file extensions in the migrations directory. The CLI commands (`generate`, `go-init`, `empty`, `dump-data`, `squash`) select the generator based on a new `migration.format` config field.

**Tech Stack:** `go.starlark.net/starlark` (Google's Starlark-go), existing `migrate` package, Viper config

---

## File Structure

### New files

| Path | Responsibility |
|---|---|
| `internal/codegen/generator.go` | `MigrationGenerator` interface + `FormatFromExtension()` helper |
| `internal/codegen/starlark_generator.go` | `StarlarkGenerator` — all `.star` codegen |
| `internal/codegen/starlark_generator_test.go` | Tests for `StarlarkGenerator` |
| `internal/interp/starlark_loader.go` | `LoadStarlarkRegistry()` — evaluate `.star` files into `*migrate.Registry` |
| `internal/interp/starlark_loader_test.go` | Tests for the Starlark loader |
| `internal/interp/starlark_builtins.go` | Starlark builtin functions (`migration`, `create_table`, `field`, etc.) |
| `internal/interp/starlark_builtins_test.go` | Tests for individual builtins |

### Modified files

| Path | What changes |
|---|---|
| `internal/config/config.go` | Add `Format string` to `MigrationConfig` (default: `"go"`) |
| `internal/config/config_test.go` | Test the new format field |
| `internal/interp/loader.go` | Rename `LoadRegistry` → `LoadGoRegistry`, add unified `LoadRegistry` that auto-detects format |
| `cmd/generate.go` | Use `MigrationGenerator` interface, select generator from config, write `.star` or `.go` |
| `cmd/go_init.go` | Use generator interface for initial migration |
| `cmd/empty.go` | Generate blank `.star` when format is starlark |
| `cmd/dump_data.go` | Generate dump-data `.star` when format is starlark |
| `go.mod` | Add `go.starlark.net` dependency |

---

## Task 1: Add `MigrationGenerator` Interface

**Files:**
- Create: `internal/codegen/generator.go`
- Modify: `internal/codegen/go_generator.go`

- [ ] **Step 1: Write the interface file**

```go
// internal/codegen/generator.go
package codegen

import "github.com/ocomsoft/morphic/internal/yaml"

// MigrationFormat identifies the output language for generated migration files.
type MigrationFormat string

const (
	FormatGo      MigrationFormat = "go"
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

// MigrationFileName returns the file name for a migration in the given format.
func MigrationFileNameForFormat(name string, format MigrationFormat) string {
	switch format {
	case FormatStarlark:
		return name + ".star"
	default:
		return name + ".go"
	}
}

// FormatFromExtension returns the format based on a file extension.
func FormatFromExtension(ext string) MigrationFormat {
	switch ext {
	case ".star":
		return FormatStarlark
	default:
		return FormatGo
	}
}

// ParseMigrationFormat parses a format string from config, defaulting to Go.
func ParseMigrationFormat(s string) MigrationFormat {
	switch s {
	case "starlark", "star":
		return FormatStarlark
	default:
		return FormatGo
	}
}
```

- [ ] **Step 2: Verify the existing `GoGenerator` already satisfies the interface**

The existing `GoGenerator` already has `GenerateMigration` with the right signature. It does NOT have `GenerateBlank` or `FileExtension`. Add them:

```go
// In go_generator.go — add these two methods:

// GenerateBlank generates a blank migration .go file with a TODO placeholder.
// This delegates to BlankGenerator for backward compatibility.
func (g *GoGenerator) GenerateBlank(name string, deps []string) (string, error) {
	bg := &BlankGenerator{}
	return bg.GenerateBlank(name, deps)
}

// FileExtension returns ".go".
func (g *GoGenerator) FileExtension() string {
	return ".go"
}
```

- [ ] **Step 3: Run tests to verify nothing breaks**

Run: `go test ./internal/codegen/ -v -count=1`
Expected: All existing tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/codegen/generator.go internal/codegen/go_generator.go
git commit -m "feat: add MigrationGenerator interface and format detection helpers"
```

---

## Task 2: Add `format` to MigrationConfig

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestConfig_MigrationFormat(t *testing.T) {
	yamlContent := `
migration:
  directory: migrations
  format: starlark
`
	tmpFile := filepath.Join(t.TempDir(), "morphic.config.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(yamlContent), 0644))

	cfg, err := Load(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "starlark", cfg.Migration.Format)
}

func TestConfig_MigrationFormatDefault(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "go", cfg.Migration.Format)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestConfig_MigrationFormat -v`
Expected: FAIL — `Format` field doesn't exist yet

- [ ] **Step 3: Add Format field to MigrationConfig**

In `internal/config/config.go`, modify `MigrationConfig`:

```go
type MigrationConfig struct {
	Directory string `yaml:"directory" mapstructure:"directory"`
	Format    string `yaml:"format" mapstructure:"format"`
}
```

And in `DefaultConfig()`, set the default:

```go
Migration: MigrationConfig{
	Directory: "migrations",
	Format:    "go",
},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add migration.format config field (default: go)"
```

---

## Task 3: Add `go.starlark.net` Dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

Run: `go get go.starlark.net`

- [ ] **Step 2: Tidy**

Run: `go mod tidy`

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: SUCCESS

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add go.starlark.net for Starlark migration support"
```

---

## Task 4: Implement Starlark Builtins

This is the core of the Starlark runtime — the functions injected into the Starlark environment that migration scripts call. Each builtin converts Starlark values into `migrate.*` structs.

**Files:**
- Create: `internal/interp/starlark_builtins.go`
- Create: `internal/interp/starlark_builtins_test.go`

- [ ] **Step 1: Write tests for the `field()` builtin**

```go
// internal/interp/starlark_builtins_test.go
package interp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func TestStarlarkBuiltin_Field_Simple(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	builtins := NewStarlarkBuiltins()

	result, err := starlark.Call(
		thread,
		builtins.Field,
		starlark.Tuple{starlark.String("id"), starlark.String("uuid")},
		[]starlark.Tuple{
			{starlark.String("primary_key"), starlark.Bool(true)},
			{starlark.String("default"), starlark.String("new_uuid")},
		},
	)
	require.NoError(t, err)

	dict, ok := result.(*starlark.Dict)
	require.True(t, ok)

	name, _, _ := dict.Get(starlark.String("name"))
	assert.Equal(t, starlark.String("id"), name)

	typ, _, _ := dict.Get(starlark.String("type"))
	assert.Equal(t, starlark.String("uuid"), typ)

	pk, _, _ := dict.Get(starlark.String("primary_key"))
	assert.Equal(t, starlark.Bool(true), pk)

	def, _, _ := dict.Get(starlark.String("default"))
	assert.Equal(t, starlark.String("new_uuid"), def)
}

func TestStarlarkBuiltin_Field_WithForeignKey(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	builtins := NewStarlarkBuiltins()

	// First create an fk value
	fkResult, err := starlark.Call(
		thread,
		builtins.FK,
		starlark.Tuple{starlark.String("auth_user")},
		[]starlark.Tuple{
			{starlark.String("on_delete"), starlark.String("PROTECT")},
		},
	)
	require.NoError(t, err)

	// Now create a field with that FK
	result, err := starlark.Call(
		thread,
		builtins.Field,
		starlark.Tuple{starlark.String("created_user_id"), starlark.String("foreign_key")},
		[]starlark.Tuple{
			{starlark.String("nullable"), starlark.Bool(true)},
			{starlark.String("foreign_key"), fkResult},
		},
	)
	require.NoError(t, err)

	dict, ok := result.(*starlark.Dict)
	require.True(t, ok)

	fk, _, _ := dict.Get(starlark.String("foreign_key"))
	fkDict, ok := fk.(*starlark.Dict)
	require.True(t, ok)

	table, _, _ := fkDict.Get(starlark.String("table"))
	assert.Equal(t, starlark.String("auth_user"), table)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interp/ -run TestStarlarkBuiltin -v`
Expected: FAIL — `NewStarlarkBuiltins` not defined

- [ ] **Step 3: Implement the builtins**

```go
// internal/interp/starlark_builtins.go
package interp

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"

	"github.com/ocomsoft/morphic/migrate"
)

// StarlarkBuiltins holds the builtin functions injected into the Starlark
// environment for migration scripts.
type StarlarkBuiltins struct {
	Migration      *starlark.Builtin
	SetDefaults    *starlark.Builtin
	SetTypeMappings *starlark.Builtin
	CreateTable    *starlark.Builtin
	DropTable      *starlark.Builtin
	AddField       *starlark.Builtin
	DropField      *starlark.Builtin
	AlterField     *starlark.Builtin
	RenameField    *starlark.Builtin
	RenameTable    *starlark.Builtin
	AddIndex       *starlark.Builtin
	DropIndex      *starlark.Builtin
	AddForeignKey  *starlark.Builtin
	DropForeignKey *starlark.Builtin
	UpsertData     *starlark.Builtin
	Field          *starlark.Builtin
	FK             *starlark.Builtin
	Index          *starlark.Builtin
	Row            *starlark.Builtin
	DefaultRef     *starlark.Builtin

	// collected holds the migration parsed from the last script evaluation.
	collected *migrate.Migration
}

// NewStarlarkBuiltins creates a new set of Starlark builtin functions.
func NewStarlarkBuiltins() *StarlarkBuiltins {
	b := &StarlarkBuiltins{}
	b.Field = starlark.NewBuiltin("field", b.fieldFn)
	b.FK = starlark.NewBuiltin("fk", b.fkFn)
	b.Index = starlark.NewBuiltin("index", b.indexFn)
	b.Row = starlark.NewBuiltin("row", b.rowFn)
	b.DefaultRef = starlark.NewBuiltin("default_ref", b.defaultRefFn)
	b.Migration = starlark.NewBuiltin("migration", b.migrationFn)
	b.SetDefaults = starlark.NewBuiltin("set_defaults", b.setDefaultsFn)
	b.SetTypeMappings = starlark.NewBuiltin("set_type_mappings", b.setTypeMappingsFn)
	b.CreateTable = starlark.NewBuiltin("create_table", b.createTableFn)
	b.DropTable = starlark.NewBuiltin("drop_table", b.dropTableFn)
	b.AddField = starlark.NewBuiltin("add_field", b.addFieldFn)
	b.DropField = starlark.NewBuiltin("drop_field", b.dropFieldFn)
	b.AlterField = starlark.NewBuiltin("alter_field", b.alterFieldFn)
	b.RenameField = starlark.NewBuiltin("rename_field", b.renameFieldFn)
	b.RenameTable = starlark.NewBuiltin("rename_table", b.renameTableFn)
	b.AddIndex = starlark.NewBuiltin("add_index", b.addIndexFn)
	b.DropIndex = starlark.NewBuiltin("drop_index", b.dropIndexFn)
	b.AddForeignKey = starlark.NewBuiltin("add_foreign_key", b.addForeignKeyFn)
	b.DropForeignKey = starlark.NewBuiltin("drop_foreign_key", b.dropForeignKeyFn)
	b.UpsertData = starlark.NewBuiltin("upsert_data", b.upsertDataFn)
	return b
}

// Collected returns the migration parsed from the last script evaluation.
func (b *StarlarkBuiltins) Collected() *migrate.Migration {
	return b.collected
}

// Env returns all builtins as a starlark.StringDict for injection into the thread.
func (b *StarlarkBuiltins) Env() starlark.StringDict {
	return starlark.StringDict{
		"migration":         b.Migration,
		"set_defaults":      b.SetDefaults,
		"set_type_mappings": b.SetTypeMappings,
		"create_table":      b.CreateTable,
		"drop_table":        b.DropTable,
		"add_field":         b.AddField,
		"drop_field":        b.DropField,
		"alter_field":       b.AlterField,
		"rename_field":      b.RenameField,
		"rename_table":      b.RenameTable,
		"add_index":         b.AddIndex,
		"drop_index":        b.DropIndex,
		"add_foreign_key":   b.AddForeignKey,
		"drop_foreign_key":  b.DropForeignKey,
		"upsert_data":       b.UpsertData,
		"field":             b.Field,
		"fk":                b.FK,
		"index":             b.Index,
		"row":               b.Row,
		"default_ref":       b.DefaultRef,
	}
}
```

The individual builtin functions (`fieldFn`, `fkFn`, `migrationFn`, etc.) convert Starlark dicts/tuples into either intermediate `*starlark.Dict` representations (for `field`, `fk`, `index`) or directly into `migrate.*` structs (for `migration`, `create_table`, etc.). The full implementations for each are detailed below.

**Key pattern:** The "leaf" builtins (`field`, `fk`, `index`, `row`) return `*starlark.Dict`. The "operation" builtins (`create_table`, `add_field`, etc.) accept those dicts and convert them to `migrate.Operation` instances, appending to an internal `[]migrate.Operation` slice. The `migration` builtin wraps everything into a `*migrate.Migration`.

Each builtin function implementation:

- `fieldFn(name, type, **kwargs)` → `*starlark.Dict` with all field properties
- `fkFn(table, **kwargs)` → `*starlark.Dict{table, on_delete, on_update}`
- `indexFn(name, fields, **kwargs)` → `*starlark.Dict{name, fields, unique, method, where}`
- `rowFn(**kwargs)` → `*starlark.Dict` (same as `map[string]any` in Go)
- `defaultRefFn(key)` → `*starlark.Dict{__default_ref__: key}`
- `migrationFn(name, deps=[], ops=[])` → collects into `b.collected`
- `setDefaultsFn(mapping)` → `&migrate.SetDefaults{}`
- `setTypeMappingsFn(mapping)` → `&migrate.SetTypeMappings{}`
- `createTableFn(name, fields=[], indexes=[])` → `&migrate.CreateTable{}`
- `dropTableFn(name)` → `&migrate.DropTable{}`
- `addFieldFn(table, field_dict)` → `&migrate.AddField{}`
- `dropFieldFn(table, field_name)` → `&migrate.DropField{}`
- `alterFieldFn(table, old_field, new_field)` → `&migrate.AlterField{}`
- `renameFieldFn(table, old_name, new_name)` → `&migrate.RenameField{}`
- `renameTableFn(old_name, new_name)` → `&migrate.RenameTable{}`
- `addIndexFn(table, index_dict)` → `&migrate.AddIndex{}`
- `dropIndexFn(table, index_name)` → `&migrate.DropIndex{}`
- `addForeignKeyFn(table, field_name, constraint_name, referenced_table, on_delete, **kwargs)` → `&migrate.AddForeignKey{}`
- `dropForeignKeyFn(table, constraint_name)` → `&migrate.DropForeignKey{}`
- `upsertDataFn(table, conflict_keys, rows)` → `&migrate.UpsertData{}`

**Conversion helpers** (private functions in the same file):

- `dictToField(dict *starlark.Dict) (migrate.Field, error)` — extracts all field properties from a Starlark dict into a `migrate.Field` struct
- `dictToIndex(dict *starlark.Dict) (migrate.Index, error)` — same for indexes
- `dictToForeignKey(dict *starlark.Dict) (*migrate.ForeignKey, error)` — same for FK
- `starlarkToGoValue(v starlark.Value) any` — converts a Starlark value (String, Int, Float, Bool, None) to the Go equivalent for use in `UpsertData.Rows`
- `stringDictToMap(dict *starlark.Dict) (map[string]string, error)` — for `set_defaults` and `set_type_mappings`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/interp/ -run TestStarlarkBuiltin -v`
Expected: PASS

- [ ] **Step 5: Write additional tests for operation builtins**

Test `create_table`, `upsert_data`, and `migration` builtins by evaluating complete Starlark scripts and checking the collected `*migrate.Migration`:

```go
func TestStarlarkBuiltin_Migration_CreateTable(t *testing.T) {
	builtins := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}

	_, err := starlark.ExecFile(thread, "test.star", `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        create_table(
            name = "users",
            fields = [
                field("id", "uuid", primary_key=True, default="new_uuid"),
                field("email", "varchar", length=255),
            ],
            indexes = [
                index("users_email_idx", ["email"], unique=True),
            ],
        ),
    ],
)
`, builtins.Env())
	require.NoError(t, err)

	m := builtins.Collected()
	require.NotNil(t, m)
	assert.Equal(t, "0001_initial", m.Name)
	assert.Empty(t, m.Dependencies)
	require.Len(t, m.Operations, 1)

	ct, ok := m.Operations[0].(*migrate.CreateTable)
	require.True(t, ok)
	assert.Equal(t, "users", ct.Name)
	require.Len(t, ct.Fields, 2)
	assert.Equal(t, "id", ct.Fields[0].Name)
	assert.True(t, ct.Fields[0].PrimaryKey)
	require.Len(t, ct.Indexes, 1)
	assert.True(t, ct.Indexes[0].Unique)
}

func TestStarlarkBuiltin_Migration_UpsertData(t *testing.T) {
	builtins := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}

	_, err := starlark.ExecFile(thread, "test.star", `
migration(
    name = "0002_seed",
    dependencies = ["0001_initial"],
    operations = [
        upsert_data(
            table = "countries",
            conflict_keys = ["code"],
            rows = [
                {"code": "AU", "name": "Australia"},
                {"code": "NZ", "name": "New Zealand"},
            ],
        ),
    ],
)
`, builtins.Env())
	require.NoError(t, err)

	m := builtins.Collected()
	require.Len(t, m.Operations, 1)
	ud, ok := m.Operations[0].(*migrate.UpsertData)
	require.True(t, ok)
	assert.Equal(t, "countries", ud.Table)
	assert.Equal(t, []string{"code"}, ud.ConflictKeys)
	require.Len(t, ud.Rows, 2)
	assert.Equal(t, "AU", ud.Rows[0]["code"])
}
```

- [ ] **Step 6: Run full test suite**

Run: `go test ./internal/interp/ -v -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/interp/starlark_builtins.go internal/interp/starlark_builtins_test.go
git commit -m "feat: implement Starlark builtins for migration DSL"
```

---

## Task 5: Implement Starlark Loader

The loader reads `.star` files from the migrations directory, evaluates each one in a fresh Starlark thread with the builtins injected, and collects the resulting `*migrate.Migration` into a `*migrate.Registry`.

**Files:**
- Create: `internal/interp/starlark_loader.go`
- Create: `internal/interp/starlark_loader_test.go`
- Modify: `internal/interp/loader.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/interp/starlark_loader_test.go
package interp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadStarlarkRegistry_SingleFile(t *testing.T) {
	dir := t.TempDir()
	script := `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        create_table(
            name = "users",
            fields = [
                field("id", "uuid", primary_key=True, default="new_uuid"),
                field("email", "varchar", length=255),
            ],
        ),
    ],
)
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(script), 0644))

	reg, err := LoadStarlarkRegistry(dir)
	require.NoError(t, err)

	migrations := reg.All()
	require.Len(t, migrations, 1)
	assert.Equal(t, "0001_initial", migrations[0].Name)
}

func TestLoadStarlarkRegistry_MultipleDeps(t *testing.T) {
	dir := t.TempDir()

	script1 := `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        set_defaults({"new_uuid": "gen_random_uuid()"}),
        create_table(name = "users", fields = [
            field("id", "uuid", primary_key=True, default="new_uuid"),
        ]),
    ],
)
`
	script2 := `
migration(
    name = "0002_add_email",
    dependencies = ["0001_initial"],
    operations = [
        add_field("users", field("email", "varchar", length=255)),
    ],
)
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(script1), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0002_add_email.star"), []byte(script2), 0644))

	reg, err := LoadStarlarkRegistry(dir)
	require.NoError(t, err)

	migrations := reg.All()
	require.Len(t, migrations, 2)
}

func TestLoadStarlarkRegistry_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadStarlarkRegistry(dir)
	require.NoError(t, err)
	assert.Empty(t, reg.All())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/interp/ -run TestLoadStarlarkRegistry -v`
Expected: FAIL — `LoadStarlarkRegistry` not defined

- [ ] **Step 3: Implement the Starlark loader**

```go
// internal/interp/starlark_loader.go
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
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		builtins := NewStarlarkBuiltins()
		thread := &starlark.Thread{Name: filepath.Base(path)}

		_, err = starlark.ExecFile(thread, filepath.Base(path), data, builtins.Env())
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", path, err)
		}

		m := builtins.Collected()
		if m == nil {
			return nil, fmt.Errorf("%s: no migration() call found", path)
		}
		reg.Register(m)
	}
	return reg, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/interp/ -run TestLoadStarlarkRegistry -v`
Expected: PASS

- [ ] **Step 5: Update unified LoadRegistry to auto-detect format**

In `internal/interp/loader.go`, rename the existing `LoadRegistry` to `LoadGoRegistry` and add a new `LoadRegistry` that detects format:

```go
// LoadRegistry auto-detects the migration format in migrationsDir and loads
// all migrations into a *migrate.Registry. If both .go and .star files exist,
// it returns an error (mixed formats are not supported).
func LoadRegistry(migrationsDir string) (*migrate.Registry, error) {
	goFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	starFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.star"))

	// Filter out main.go from go files count
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
```

- [ ] **Step 6: Update all callers of old `LoadRegistry`**

Search for all calls to `interp.LoadRegistry` — they should continue working unchanged since the new `LoadRegistry` delegates to `LoadGoRegistry` for `.go`-only dirs.

Run: `grep -rn "interp.LoadRegistry" cmd/`

Verify the calls are: `cmd/generate.go`, `cmd/migrate.go`. They should work unchanged.

- [ ] **Step 7: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/interp/starlark_loader.go internal/interp/starlark_loader_test.go internal/interp/loader.go
git commit -m "feat: add Starlark migration loader with auto-format detection"
```

---

## Task 6: Implement StarlarkGenerator (Codegen)

This generates `.star` source files from `yaml.SchemaDiff`, mirroring `GoGenerator` but outputting Starlark syntax.

**Files:**
- Create: `internal/codegen/starlark_generator.go`
- Create: `internal/codegen/starlark_generator_test.go`

- [ ] **Step 1: Write the failing test for CreateTable**

```go
// internal/codegen/starlark_generator_test.go
package codegen

import (
	"testing"

	"github.com/ocomsoft/morphic/internal/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestStarlarkGenerator_CreateTable(t *testing.T) {
	gen := NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type: yaml.ChangeTypeTableAdded,
				NewValue: yaml.Table{
					Name: "users",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid", Nullable: boolPtr(false)},
						{Name: "email", Type: "varchar", Length: 255, Nullable: boolPtr(true)},
					},
				},
			},
		},
	}

	src, err := gen.GenerateMigration("0001_initial", nil, diff, nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, src, `migration(`)
	assert.Contains(t, src, `name = "0001_initial"`)
	assert.Contains(t, src, `create_table(`)
	assert.Contains(t, src, `name = "users"`)
	assert.Contains(t, src, `field("id", "uuid", primary_key=True, default="new_uuid")`)
	assert.Contains(t, src, `field("email", "varchar", nullable=True, length=255)`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codegen/ -run TestStarlarkGenerator -v`
Expected: FAIL — `NewStarlarkGenerator` not defined

- [ ] **Step 3: Implement StarlarkGenerator**

The generator follows the same `generateOperation` switch pattern as `GoGenerator`, but emits Starlark syntax with named arguments. Key differences from Go output:

- No `package`/`import`/`func init()` wrapper — just a bare `migration()` call
- Uses Python-style `True`/`False`/`None` for booleans/nil
- Uses named arguments: `field("name", "type", nullable=True, length=255)`
- Uses `#` for comments (REVIEW markers)
- No `go/format` step needed — output is already well-formatted text

```go
// internal/codegen/starlark_generator.go
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ocomsoft/morphic/internal/utils"
	"github.com/ocomsoft/morphic/internal/yaml"
)

// StarlarkGenerator produces .star source code for migration files.
type StarlarkGenerator struct{}

// NewStarlarkGenerator creates a new StarlarkGenerator instance.
func NewStarlarkGenerator() *StarlarkGenerator {
	return &StarlarkGenerator{}
}

// FileExtension returns ".star".
func (g *StarlarkGenerator) FileExtension() string {
	return ".star"
}

// GenerateBlank generates a blank Starlark migration with a TODO comment.
func (g *StarlarkGenerator) GenerateBlank(name string, deps []string) (string, error) {
	var b strings.Builder
	b.WriteString("migration(\n")
	b.WriteString(fmt.Sprintf("    name = %q,\n", name))
	b.WriteString(fmt.Sprintf("    dependencies = [%s],\n", g.formatDepsList(deps)))
	b.WriteString("    operations = [\n")
	b.WriteString("        # TODO: Add migration operations here.\n")
	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// GenerateMigration generates a .star file from a schema diff.
func (g *StarlarkGenerator) GenerateMigration(
	name string,
	deps []string,
	diff *yaml.SchemaDiff,
	currentSchema, previousSchema *yaml.Schema,
	decisions map[int]yaml.PromptResponse,
) (string, error) {
	// ... implementation follows same pattern as GoGenerator.GenerateMigration
	// but emits Starlark syntax
}
```

The operation emitters follow this pattern for each type:

- `generateCreateTable` → `create_table(name = "...", fields = [...], indexes = [...])`
- `generateDropTable` → `drop_table(name = "...", schema_only=True)`
- `generateAddField` → `add_field("table", field("name", "type", ...))`
- `generateSetDefaults` → `set_defaults({...})`
- etc.

Field literal helper: `generateStarlarkField(f yaml.Field) string` outputs `field("name", "type", primary_key=True, nullable=True, default="new_uuid", length=255)` — only including non-default kwargs.

Index literal helper: `generateStarlarkIndex(idx yaml.Index) string` outputs `index("name", ["field1", "field2"], unique=True)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/codegen/ -run TestStarlarkGenerator -v`
Expected: PASS

- [ ] **Step 5: Write tests for all remaining operation types**

Add tests for: `AddField`, `DropField`, `AlterField`, `RenameField`, `RenameTable`, `AddIndex`, `DropIndex`, `AddForeignKey`, `DropForeignKey`, `SetDefaults`, `SetTypeMappings`, `UpsertData`, `GenerateBlank`, `MultipleDependencies`, `SchemaOnly`, `IgnoreErrors`, `ReviewComment`.

Each test follows the same pattern: build a `yaml.SchemaDiff` with one change, call `GenerateMigration`, assert the output contains the expected Starlark syntax.

- [ ] **Step 6: Run full codegen test suite**

Run: `go test ./internal/codegen/ -v -count=1`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/codegen/starlark_generator.go internal/codegen/starlark_generator_test.go
git commit -m "feat: implement StarlarkGenerator for .star migration codegen"
```

---

## Task 7: Wire Format Selection into CLI Commands

Update the CLI commands to select the appropriate generator based on config.

**Files:**
- Modify: `cmd/generate.go`
- Modify: `cmd/go_init.go`
- Modify: `cmd/empty.go`
- Modify: `cmd/dump_data.go`

- [ ] **Step 1: Create a helper to instantiate the correct generator**

Add to `cmd/generate.go` (or a new `cmd/helpers.go`):

```go
// newGenerator returns the appropriate MigrationGenerator based on the config format.
func newGenerator(cfg *config.Config) codegen.MigrationGenerator {
	format := codegen.ParseMigrationFormat(cfg.Migration.Format)
	switch format {
	case codegen.FormatStarlark:
		return codegen.NewStarlarkGenerator()
	default:
		return codegen.NewGoGenerator()
	}
}

// migrationFileName returns the correct filename for the configured format.
func migrationFileName(name string, cfg *config.Config) string {
	format := codegen.ParseMigrationFormat(cfg.Migration.Format)
	return codegen.MigrationFileNameForFormat(name, format)
}
```

- [ ] **Step 2: Update `runGoMakeMigrations` in `cmd/generate.go`**

Replace:
```go
gen := codegen.NewGoGenerator()
```
With:
```go
gen := newGenerator(cfg)
```

Replace:
```go
outPath := filepath.Join(migrationsDir, codegen.MigrationFileName(name))
```
With:
```go
outPath := filepath.Join(migrationsDir, migrationFileName(name, cfg))
```

- [ ] **Step 3: Update `ExecuteGoMigrationInit` in `cmd/go_init.go`**

Same pattern — use `newGenerator(cfg)` and `migrationFileName`. Note: `main.go` and `go.mod` generation should only happen when format is Go. For Starlark format, skip those files.

- [ ] **Step 4: Update `cmd/empty.go`**

Replace `BlankGenerator` usage with `gen.GenerateBlank()` from the interface. Use `migrationFileName` for the output path.

- [ ] **Step 5: Update `cmd/dump_data.go`**

The `DumpDataGenerator` needs a Starlark equivalent. Add a `GenerateDumpData` method to the `StarlarkGenerator` that emits `upsert_data()` operations. Update `cmd/dump_data.go` to use the configured format.

- [ ] **Step 6: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 7: Run linter**

Run: `golangci-lint run --no-config ./...`
Expected: No new issues

- [ ] **Step 8: Commit**

```bash
git add cmd/generate.go cmd/go_init.go cmd/empty.go cmd/dump_data.go
git commit -m "feat: wire format selection into CLI commands (go/starlark)"
```

---

## Task 8: Round-Trip Integration Test

Verify the full pipeline: generate a `.star` file from a schema diff, then load it back with the Starlark loader and verify the resulting `*migrate.Migration` matches.

**Files:**
- Create: `internal/codegen/roundtrip_test.go`

- [ ] **Step 1: Write the round-trip test**

```go
// internal/codegen/roundtrip_test.go
package codegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarlark_RoundTrip_CreateTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	nullable := true
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type: yaml.ChangeTypeTableAdded,
				NewValue: yaml.Table{
					Name: "users",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid", Nullable: &nullable},
						{Name: "email", Type: "varchar", Length: 255, Nullable: &nullable},
					},
					Indexes: []yaml.Index{
						{Name: "users_email_idx", Fields: []string{"email"}, Unique: true},
					},
				},
			},
		},
	}

	// Generate
	src, err := gen.GenerateMigration("0001_initial", nil, diff, nil, nil, nil)
	require.NoError(t, err)

	// Write to temp dir
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src), 0644))

	// Load back
	reg, err := interp.LoadStarlarkRegistry(dir)
	require.NoError(t, err)

	migrations := reg.All()
	require.Len(t, migrations, 1)

	m := migrations[0]
	assert.Equal(t, "0001_initial", m.Name)
	assert.Empty(t, m.Dependencies)
	require.Len(t, m.Operations, 1)
}

func TestStarlark_RoundTrip_UpsertData(t *testing.T) {
	// Similar: generate a dump-data .star, load it, verify rows match
}

func TestStarlark_RoundTrip_FullMigration(t *testing.T) {
	// Exercise: SetDefaults + SetTypeMappings + CreateTable + AddForeignKey
	// Generate, write, load, verify all operations round-trip correctly
}
```

- [ ] **Step 2: Run the round-trip tests**

Run: `go test ./internal/codegen/ -run TestStarlark_RoundTrip -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/codegen/roundtrip_test.go
git commit -m "test: add Starlark round-trip integration tests"
```

---

## Task 9: End-to-End Test with Real Schema

Test against the actual AirRadiators-style schema to verify the pipeline works with realistic data.

**Files:**
- Create: `internal/codegen/e2e_starlark_test.go`

- [ ] **Step 1: Write an e2e test**

Build a `yaml.SchemaDiff` that mimics a real migration (SetDefaults + CreateTable with FK fields + indexes), generate to `.star`, load back, and verify all operations are present with correct field types, lengths, FK references, and index definitions.

- [ ] **Step 2: Run the e2e test**

Run: `go test ./internal/codegen/ -run TestE2E_Starlark -v`
Expected: PASS

- [ ] **Step 3: Run full test suite and linter**

Run: `go test ./... -count=1 && golangci-lint run --no-config ./...`
Expected: All PASS, no new lint issues

- [ ] **Step 4: Commit**

```bash
git add internal/codegen/e2e_starlark_test.go
git commit -m "test: add e2e Starlark migration test with realistic schema"
```

---

## Task 10: Documentation

**Files:**
- Modify: `cmd/root.go` (update help text to mention format options)

- [ ] **Step 1: Update root command description**

Add mention of Starlark support and the `migration.format` config option to the root command's Long description.

- [ ] **Step 2: Add `--format` flag to generate command**

Add an optional `--format` flag to `cmd/generate.go` that overrides the config file setting for one-off generation:

```go
generateCmd.Flags().StringVar(&goMigFormat, "format", "", "Output format: go or starlark (overrides config)")
```

When set, override `cfg.Migration.Format` before calling `newGenerator`.

- [ ] **Step 3: Run tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/root.go cmd/generate.go
git commit -m "docs: add Starlark format documentation and --format flag"
```
