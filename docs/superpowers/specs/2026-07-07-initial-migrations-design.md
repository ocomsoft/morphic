# Initial Migrations (`initial = True` + `--fake-initial`) Design Spec

## Goal

Add Django-style `initial = True` migration support so that morphic can adopt
an existing database without re-running table creation SQL. When
`morphic migrate up --fake-initial` is used, migrations marked as initial are
automatically faked if their tables (and expected columns) already exist in the
live database.

## Architecture

The feature has four components:

1. **Migration metadata** — `Initial bool` field on `migrate.Migration` and
   `initial` kwarg on the Starlark `migration()` builtin
2. **Provider introspection** — `TableColumns(db, tableName)` method on the
   Provider interface for checking table/column existence
3. **Runner logic** — `--fake-initial` flag on `migrate up` that auto-fakes
   initial migrations when their schema already exists
4. **Generator/converter support** — preserve `initial = True` through
   `from-makemigrations`, squash, and convert workflows

## Detailed Design

### 1. Migration Struct

Add `Initial bool` to `migrate.Migration` in `migrate/types.go`:

```go
type Migration struct {
    Name         string      `json:"name"`
    Dependencies []string    `json:"dependencies"`
    Operations   []Operation `json:"-"`
    Replaces     []string    `json:"replaces,omitempty"`
    Initial      bool        `json:"initial,omitempty"`
}
```

### 2. Starlark DSL

The `migration()` builtin in `internal/interp/starlark_builtins.go` gains an
optional `initial` kwarg:

```python
migration(
    name = "0001_initial",
    dependencies = [],
    initial = True,
    operations = [
        create_table("users",
            fields = [
                field("id", "uuid", primary_key=True, default="new_uuid"),
                field("email", "varchar", length=255),
            ],
        ),
    ],
)
```

- `initial` is optional, defaults to `False`
- Explicitly set by the user — never auto-detected or auto-generated
- The kwarg is a `starlark.Bool`, mapped to `m.Initial`

### 3. Provider Interface: `TableColumns`

Add to `internal/providers/provider.go`:

```go
// TableColumns returns the column names for the given table in the database.
// Returns nil, nil if the table does not exist (not an error).
// Returns nil, err on query failure.
TableColumns(db *sql.DB, tableName string) ([]string, error)
```

**PostgreSQL implementation** (`internal/providers/postgresql/provider.go`):

```sql
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position
```

- Returns `[]string{"id", "email", "created_at"}` if the table exists
- Returns `nil, nil` if the table does not exist

**Other providers** (MySQL, SQLite, SQL Server, etc.) return
`nil, fmt.Errorf("fake-initial is not supported for this database type")`.
Implementations can be added incrementally.

### 4. Runner: `--fake-initial` Logic

`RunOptions` gains `FakeInitial bool`:

```go
type RunOptions struct {
    WarnOnMissingDrop bool
    FakeInitial       bool
}
```

In `Runner.Up()`, when `opts.FakeInitial` is true and a pending migration has
`m.Initial == true`:

1. **Extract expected schema** — walk `m.Operations` and collect all
   `*CreateTable` operations. For each, record the table name and the field
   names from its `Fields` slice.
2. **Query live database** — for each expected table, call
   `r.provider.TableColumns(r.db, tableName)`.
3. **Compare** — the migration is fakeable if:
   - ALL expected tables exist in the database
   - For each table, ALL expected column names are present in the live columns
   - Extra columns in the live database are allowed (the DB may have been
     extended manually or by a later migration)
4. **Decision:**
   - If all checks pass: fake the migration (call `r.recorder.RecordApplied`)
     and print `Applying 0001_initial... faked (tables already exist)`
   - If any table is missing or any expected column is missing: apply normally
     (run the SQL as usual)

Migrations where `m.Initial` is false are always applied normally, regardless
of the `--fake-initial` flag.

### 5. CLI Integration

In `migrate/app.go`, add `--fake-initial` flag to `buildUpCommand()`:

```go
var fakeInitial bool
cmd.Flags().BoolVar(&fakeInitial, "fake-initial", false,
    "Fake initial migrations if their tables already exist in the database")
```

Pass to `RunOptions{FakeInitial: fakeInitial}`.

No changes needed to `cmd/migrate.go` — it uses `DisableFlagParsing` and
forwards all args to the inner app.

### 6. Starlark Converter

In `internal/codegen/starlark_converter.go`, `ConvertMigrationToStarlark()`
emits `initial = True` after the `dependencies` line when `m.Initial` is set.
This preserves the flag through `from-makemigrations` and squash workflows.

### 7. Generator Header

All Starlark generators that emit `migration(...)` calls support the `Initial`
field. Since the user chose "always manual", generators do NOT auto-set
`initial = True` — they only emit it when explicitly requested (e.g., via a
future `--initial` flag on `morphic generate`, which is out of scope for this
spec).

## Verification Behavior

| Scenario | Result |
|----------|--------|
| `--fake-initial`, migration has `initial=True`, all tables+columns exist | Faked |
| `--fake-initial`, migration has `initial=True`, one table missing | Applied normally |
| `--fake-initial`, migration has `initial=True`, table exists but column missing | Applied normally |
| `--fake-initial`, migration has `initial=True`, extra columns in live DB | Faked (lenient) |
| `--fake-initial`, migration has `initial=False` | Applied normally |
| No `--fake-initial` flag, migration has `initial=True` | Applied normally |
| `--fake-initial`, provider doesn't support `TableColumns` | Error |

## Out of Scope

- Auto-detecting initial migrations from their shape (no deps, only CreateTable)
- Auto-setting `initial = True` during `morphic generate`
- `--fake-initial` as a standalone command
- Full schema verification (types, indexes, constraints) — `db-diff` covers this
- MySQL/SQLite/SQL Server `TableColumns` implementations (stubs with error)

## Files Affected

| File | Change |
|------|--------|
| `migrate/types.go` | Add `Initial bool` to Migration |
| `migrate/runner.go` | `FakeInitial` in RunOptions, fake-initial logic in `Up()` |
| `migrate/app.go` | `--fake-initial` flag on `buildUpCommand()` |
| `internal/interp/starlark_builtins.go` | `initial` kwarg on `migration()` |
| `internal/providers/provider.go` | `TableColumns()` method on Provider interface |
| `internal/providers/postgresql/provider.go` | PostgreSQL `TableColumns()` implementation |
| `internal/providers/*/provider.go` | Stub `TableColumns()` returning unsupported error |
| `internal/codegen/starlark_converter.go` | Emit `initial = True` in converter |
| `internal/codegen/squash_generator.go` | Preserve `initial = True` when squashing initial migrations |
| `docs/commands/migrate.md` | Document `--fake-initial` flag |
