# empty Command

The `empty` command creates a blank migration `.go` file with no operations. Use this to write custom migrations that are not generated automatically from schema changes — for example, data backfills, custom SQL triggers, or any operation the diff engine cannot detect.

This is the Go equivalent of Django's `morphic --empty` flag.

## Overview

Running `morphic generate empty` generates a migration file that:

- Has an empty `Operations` slice with a `TODO` comment as a placeholder
- Automatically depends on the current DAG leaves (the most recently created migrations), so it slots into the chain correctly
- Is named with the next sequential number and a custom name suffix

## Usage

```
morphic generate empty [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `blank` | Custom migration name suffix |
| `--dry-run` | bool | `false` | Print the generated migration source without writing a file |
| `--verbose` | bool | `false` | Show detailed output (migration name, dependencies) |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to the configuration file |

---

## Generated File

The generated migration file contains an empty `Operations` slice with a placeholder comment:

```go
package main

import m "github.com/ocomsoft/morphic/migrate"

func init() {
    m.Register(&m.Migration{
        Name:         "0003_add_custom_triggers",
        Dependencies: []string{"0002_add_users"},
        Operations: []m.Operation{
            // TODO: Add migration operations here.
            // Example: &m.RunSQL{ForwardSQL: "SELECT 1", BackwardSQL: ""},
        },
    })
}
```

Edit the file and replace the TODO comment with your custom operations.

---

## Examples

### Create a blank migration with a custom name

```bash
morphic generate empty --name add_custom_triggers

# Output:
# Created migrations/0003_add_custom_triggers.go
# Edit the file and add your migration operations.
```

### Preview without writing

```bash
morphic generate empty --name backfill_users --dry-run
```

### Verbose output (shows dependencies)

```bash
morphic generate empty --name seed_data --verbose

# Output:
# Generating blank migration: 0004_seed_data
# Dependencies: [0003_add_users]
# Created migrations/0004_seed_data.go
# Edit the file and add your migration operations.
```

---

## Filling in Operations

After generating the blank migration, open the file and add your operations. Common patterns:

### Raw SQL

```go
Operations: []m.Operation{
    &m.RunSQL{
        ForwardSQL:  "CREATE TRIGGER ...",
        BackwardSQL: "DROP TRIGGER ...",
    },
},
```

### Data backfill (schema-only, no DB changes)

```go
Operations: []m.Operation{
    &m.RunSQL{
        ForwardSQL:  "UPDATE users SET status = 'active' WHERE status IS NULL",
        BackwardSQL: "",
    },
},
```

### Multiple operations

```go
Operations: []m.Operation{
    &m.RunSQL{
        ForwardSQL:  "CREATE INDEX CONCURRENTLY idx_users_email ON users (email)",
        BackwardSQL: "DROP INDEX idx_users_email",
    },
    &m.RunSQL{
        ForwardSQL:  "COMMENT ON TABLE users IS 'Application users'",
        BackwardSQL: "",
    },
},
```

### Seed reference data with UpsertData

`UpsertData` generates the correct upsert SQL automatically for your target database — no raw SQL needed:

```go
Operations: []m.Operation{
    &m.UpsertData{
        Table:        "countries",
        ConflictKeys: []string{"code"},
        Rows: []map[string]any{
            {"code": "AU", "name": "Australia"},
            {"code": "US", "name": "United States"},
            {"code": "NZ", "name": "New Zealand"},
        },
    },
},
```

Rollback (`down`) automatically generates `DELETE` statements matching the conflict keys.

See the [migrate operations reference](../migrations.md) for the full list of available operation types.

---

## Workflow

```
morphic generate empty --name my_custom_migration
↓
Edit migrations/NNNN_my_custom_migration.go
  → Add operations
↓
morphic migrate up
```

---

## See Also

- [morphic command](./morphic.md) — Generate migrations from schema changes
- [migrate command](./migrate.md) — Run `up`, `down`, `status`, `fake` etc.
- [Migrations Guide](../migrations.md) — Full guide to the migration framework
- [init command](./init.md) — Initialise the migrations directory
