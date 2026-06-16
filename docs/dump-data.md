# dump-data

## Purpose

The `dump-data` command connects to a live database, reads all rows from the
specified tables, and generates a migration file containing `UpsertData`
operations for each table. This is useful for seeding reference or lookup data
(e.g. country codes, unit types, roles) so that the data can be version-controlled
and applied consistently across environments via the normal migration workflow.

The output format (Go or Starlark) is determined by the `migration.format` setting
in your `makemigrations.config.yaml`. When Starlark format is active, `dump-data`
emits a `.star` file using `upsert_data()` instead of a `.go` file with `UpsertData`.

## Usage

```
morphic generate dump-data [table1 table2 ...] [flags]
```

## Flags

| Flag             | Type     | Default     | Description                                                                                      |
|------------------|----------|-------------|--------------------------------------------------------------------------------------------------|
| `--name`         | string   | `""`        | Custom migration name suffix                                                                     |
| `--dry-run`      | bool     | `false`     | Print generated source without writing                                                           |
| `--verbose`      | bool     | `false`     | Show connection and row-count details                                                            |
| `--jsonl`        | bool     | `false`     | Write row data to JSONL files instead of embedding inline                                        |
| `--conflict-key` | []string | (auto)      | PK columns for ON CONFLICT; applied to all tables; required if table not in migration schema     |
| `--where`        | []string | (none)      | WHERE filter; use `table:condition` for per-table or just `condition` for all                     |
| `--dsn`          | string   | `""`        | Full database DSN (overrides host/port/etc.)                                                     |
| `--host`         | string   | `localhost` | Database host                                                                                    |
| `--port`         | int      | (varies)    | Database port                                                                                    |
| `--database`     | string   | `""`        | Database name                                                                                    |
| `--username`     | string   | `""`        | Database username                                                                                |
| `--password`     | string   | `""`        | Database password                                                                                |
| `--sslmode`      | string   | `disable`   | SSL mode (PostgreSQL)                                                                            |

## How PK detection works

- Primary-key columns are read from the migration **SchemaState** — the
  reconstructed schema built by walking your existing migration chain.
- The `--conflict-key` flag overrides automatic detection and applies the
  specified column(s) to **all** tables in the invocation.
- If the table does not yet exist in the migration schema (i.e. you have not
  generated migrations for it), you **must** supply `--conflict-key` so the
  generated `UpsertData` operation knows which columns form the conflict target.

## Generated output

Running the command produces a standard Go migration file. For example,
dumping a `unit_type` table with two rows generates code similar to:

```go
func init() {
    m.Register(&m.Migration{
        Name: "0003_dump_unit_type",
        Operations: []m.Operation{
            &m.UpsertData{
                Table:       "unit_type",
                ConflictKey: []string{"id"},
                Columns:     []string{"id", "name", "code"},
                Rows: [][]interface{}{
                    {1, "Metric", "MET"},
                    {2, "Imperial", "IMP"},
                },
            },
        },
    })
}
```

Each `UpsertData` operation translates to an `INSERT ... ON CONFLICT (pk) DO UPDATE`
statement at migration runtime.

## --jsonl: Storing Row Data in Separate Files

Write row data to JSONL files in `migrations/data/` instead of embedding it inline
in the migration source. This keeps migration files small and makes large reference
datasets (country codes, postal codes, product catalogs) reviewable and diff-able
as standalone files.

```bash
# Write data to JSONL files
morphic generate dump-data countries --jsonl

# Preview the migration (data files are still written)
morphic generate dump-data countries --jsonl --dry-run
```

When `--jsonl` is used:
- Data files are written to `migrations/data/<migration_name>_<table>.jsonl`
- The `data/` subdirectory is created automatically if it does not exist
- The generated Go migration references the JSONL file via the `DataFile` field on `UpsertData`
- The generated Starlark migration references the file via the `file=` kwarg on `upsert_data()`

Example Go output with `--jsonl`:

```go
&m.UpsertData{
    Table:       "countries",
    ConflictKey: []string{"code"},
    DataFile:    "data/0004_dump_countries_countries.jsonl",
},
```

Example Starlark output with `--jsonl`:

```starlark
upsert_data("countries",
    conflict_keys = ["code"],
    file = "data/0004_dump_countries_countries.jsonl",
)
```

The JSONL file format contains one JSON object per line:

```
{"code":"AU","name":"Australia","population":25687041}
{"code":"US","name":"United States","population":331002651}
```

Empty lines and lines beginning with `//` are treated as comments and skipped at
migration runtime. All objects in the file must share the same set of keys.

## Examples

```bash
# Seed a single reference table
morphic generate dump-data unit_type

# Seed multiple tables at once
morphic generate dump-data unit_type currency --name seed_reference_data

# Preview without writing
morphic generate dump-data roles --dry-run

# Override conflict key (table not in schema yet)
morphic generate dump-data legacy_table --conflict-key id

# Specify database connection explicitly
morphic generate dump-data countries --dsn "host=prod-db port=5432 dbname=myapp user=ro sslmode=require"

# Write row data to separate JSONL files (recommended for large datasets)
morphic generate dump-data countries --jsonl

# Preview JSONL-backed output without committing to disk
morphic generate dump-data countries --jsonl --dry-run
```

## Filtering Rows with --where

By default, all rows are fetched. Use `--where` to filter:

```bash
# Per-table filter
morphic generate dump-data users orders --where "users:status='active'" --where "orders:total > 0"

# Global filter (applies to all tables)
morphic generate dump-data users orders --where "active = 1"

# Multiple conditions combined with AND
morphic generate dump-data users --where "users:status='active'" --where "users:created_at > '2025-01-01'"
```

## Limitations

- The `--where` condition is appended to the query as-is. Ensure the condition is valid SQL for your target database.
- `--conflict-key` applies to **all** tables in one invocation. If tables have
  different primary keys, run the command separately for each table.
- Values are stored as plain Go literals (strings, ints, `nil`). SQL quoting
  and escaping happen at migration runtime, not at generation time.
