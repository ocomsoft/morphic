# dump-data Command

The `dump-data` command connects to a live database, fetches rows from the specified tables, and generates a Go migration file containing `UpsertData` operations. Use this to seed reference or lookup data (e.g. country codes, roles, unit types) so the data is version-controlled and applied consistently across environments via the normal migration workflow.

## Overview

Running `morphic generate dump-data <table1> [table2 ...]` generates a migration file that:

- Contains one `UpsertData` operation per table
- Uses `INSERT ... ON CONFLICT (pk) DO UPDATE` at migration runtime
- Automatically depends on the current DAG leaves so it slots into the migration chain correctly
- Detects primary keys from the migration SchemaState (reconstructed from existing migrations)

## Usage

```
morphic generate dump-data [table1 table2 ...] [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `""` | Custom migration name suffix |
| `--dry-run` | bool | `false` | Print generated migration without writing |
| `--verbose` | bool | `false` | Show connection and row-count details |
| `--conflict-key` | []string | (auto) | Override PK detection; applied to all tables |
| `--where` | []string | (none) | WHERE filter; use `table:condition` for per-table or just `condition` for all |
| `--dsn` | string | `""` | Full database DSN (overrides host/port/etc.) |
| `--host` | string | `localhost` | Database host |
| `--port` | int | (varies) | Database port |
| `--database` | string | `""` | Database name |
| `--username` | string | `""` | Database username |
| `--password` | string | `""` | Database password |
| `--sslmode` | string | `disable` | SSL mode (PostgreSQL) |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to the configuration file |

---

## How PK Detection Works

1. Primary-key columns are read from the migration **SchemaState** — the reconstructed schema built by walking your existing migration chain.
2. The `--conflict-key` flag overrides automatic detection and applies the specified column(s) to **all** tables in the invocation.
3. If the table does not yet exist in the migration schema (i.e. you have not generated migrations for it), you **must** supply `--conflict-key`.

---

## Database Connection

Connection details are resolved in this order:

1. `--dsn` flag (highest priority)
2. `DATABASE_URL` environment variable
3. Individual flags (`--host`, `--port`, `--database`, `--username`, `--password`, `--sslmode`)
4. Config file defaults

Supported databases: PostgreSQL, MySQL, TiDB, SQLite.

---

## Filtering Rows with --where

By default, `dump-data` fetches **all rows** from each table. Use `--where` to filter which rows are included in the generated migration.

### Per-table filter

Prefix the condition with the table name and a colon:

```bash
morphic generate dump-data users orders --where "users:status='active'" --where "orders:total > 0"
```

This fetches only active users and orders with a positive total.

### Global filter (all tables)

Omit the table prefix to apply the condition to every table:

```bash
morphic generate dump-data users orders --where "active = 1"
```

### Combining filters

Multiple `--where` entries for the same table are combined with `AND`:

```bash
morphic generate dump-data users --where "users:status='active'" --where "users:created_at > '2025-01-01'"
# Equivalent to: WHERE status='active' AND created_at > '2025-01-01'
```

---

## Generated Output

The command produces a standard Go migration file. For example, dumping a `unit_type` table with two rows generates:

```go
package main

import m "github.com/ocomsoft/morphic/migrate"

func init() {
    m.Register(&m.Migration{
        Name:         "0003_dump_unit_type",
        Dependencies: []string{"0002_add_indexes"},
        Operations: []m.Operation{
            &m.UpsertData{
                Table:        "unit_type",
                ConflictKeys: []string{"id"},
                Rows: []map[string]any{
                    {"code": "MET", "id": int64(1), "name": "Metric"},
                    {"code": "IMP", "id": int64(2), "name": "Imperial"},
                },
            },
        },
    })
}
```

Each `UpsertData` operation translates to an `INSERT ... ON CONFLICT (pk) DO UPDATE` statement at migration runtime. Rollback (`down`) generates `DELETE` statements matching the conflict keys.

---

## Examples

### Dump a single table

```bash
morphic generate dump-data countries
```

### Dump multiple tables with a custom name

```bash
morphic generate dump-data countries currencies --name seed_reference_data
```

### Preview without writing

```bash
morphic generate dump-data roles --dry-run
```

### Override conflict key (table not in schema yet)

```bash
morphic generate dump-data legacy_table --conflict-key id
```

### Specify database connection via DSN

```bash
morphic generate dump-data countries --dsn "host=prod-db port=5432 dbname=myapp user=ro sslmode=require"
```

### Dump only active records

```bash
morphic generate dump-data users --where "users:status='active'" --dry-run
```

### Filter all tables with the same condition

```bash
morphic generate dump-data countries currencies --where "active = 1"
```

### Verbose output

```bash
morphic generate dump-data countries --verbose

# Output:
# Connecting to postgresql database...
# Fetching rows from "countries" (conflict keys: [id])...
#   42 rows fetched
# Generating migration: 0005_dump_countries
# Dependencies: [0004_add_indexes]
# Created migrations/0005_dump_countries.go
```

---

## Limitations

- `--conflict-key` applies to **all** tables in one invocation. If tables have different primary keys, run the command separately for each table.
- Values are stored as plain Go literals (strings, ints, `nil`). SQL quoting and escaping happen at migration runtime, not at generation time.
- The `--where` condition is appended to the query as-is. Ensure the condition is valid SQL for your target database.

---

## See Also

- [empty command](./empty.md) — Create a blank migration for custom operations
- [morphic command](./morphic.md) — Generate migrations from schema changes
- [migrate command](./migrate.md) — Run `up`, `down`, `status`, `fake` etc.
- [Migrations Guide](../migrations.md) — Full guide to the Go migration framework
