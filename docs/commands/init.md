# init Command

The `init` command initialises a new morphic project. By default it sets up the **migration framework** — type-safe migration `.star` files that the morphic CLI runs in-process via the Starlark interpreter. A legacy YAML-to-SQL workflow is available via the `--sql` flag.

## Overview

Running `morphic init` bootstraps everything needed to start writing migrations:

- Creates the `migrations/` directory
- If an existing `migrations/.schema_snapshot.yaml` is found, generates `migrations/0001_initial.star` with `create_table` calls for every table already defined in that snapshot, and prints instructions for fake-applying it

If no snapshot is found the command creates an empty setup and prints instructions for generating the first migration.

## Usage

```
morphic init [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database` | string | `postgresql` | Target database type — influences generated config. Supported values: `postgresql`, `mysql`, `sqlite`, `sqlserver` |
| `--verbose` | bool | `false` | Print detailed output during initialisation |
| `--sql` | bool | `false` | Use the legacy YAML-to-SQL workflow instead of generating Starlark migration files |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to the configuration file |

---

## Starlark Migration Workflow (Default)

### What Gets Created

```
project/
└── migrations/
    └── 0001_initial.star # Only created when .schema_snapshot.yaml is found
```

### `migrations/0001_initial.star` (snapshot import)

When `migrations/.schema_snapshot.yaml` already exists at init time, `init` reads every table defined in the snapshot and produces a migration file with a `create_table` call for each one:

```python
migration(
    name = "0001_initial",
    operations = [
        create_table("users", fields = [
            uuid("id", primary_key = True),
            string("email", length = 255, nullable = False),
            timestamp("created_at", auto_create = True),
        ]),
        create_table("orders", fields = [
            uuid("id", primary_key = True),
            foreign_key("user_id", table = "users", on_delete = "CASCADE", nullable = False),
            integer("quantity", default = 1),
        ]),
    ],
)
```

---

## Examples

### Fresh Project — No Existing Schema

```bash
# Initialise a new migration project (PostgreSQL default)
morphic init

# Output:
# Initialization complete. No existing schema found.
#
# To generate your first migration:
#   morphic generate --name "initial"
#
# Then run:
#   morphic migrate up
#
# Migrations are interpreted in-process via the Starlark interpreter.
```

Step-by-step after a fresh init:

```bash
# 1. Generate your first migration
morphic generate --name "initial"

# 2. Apply migrations (Starlark interpreter loads the .star files in-process)
morphic migrate up
```

### Existing Project — Snapshot Found

When a `migrations/.schema_snapshot.yaml` already exists (for example when migrating an existing project to the Starlark workflow):

```bash
morphic init

# Output:
# Created migrations/0001_initial.star (from existing schema snapshot)
#
# Your database already has these tables applied.
# Mark this migration as applied without re-running SQL:
#
#   morphic migrate fake 0001_initial
```

Step-by-step after a snapshot-based init:

```bash
# 1. Mark the initial migration as already applied (schema already in DB)
morphic migrate fake 0001_initial

# 2. Confirm status
morphic migrate status
```

### Initialise for a Different Database

```bash
# MySQL
morphic init --database mysql

# SQLite
morphic init --database sqlite

# SQL Server
morphic init --database sqlserver
```

### Verbose Output

```bash
morphic init --verbose

# Output includes per-file creation details, snapshot parsing steps,
# and table counts when 0001_initial.go is generated.
```

---

## Post-Init Workflow Summary

| Scenario | Commands |
|----------|----------|
| Fresh project | `morphic generate --name "initial"` → `morphic migrate up` |
| Existing DB (snapshot found) | `morphic migrate fake 0001_initial` → `morphic migrate status` |

---

## Legacy SQL Workflow (`--sql`)

The `--sql` flag opts into the original YAML-to-SQL workflow. No Starlark files are generated. Use this only if you are maintaining a project that was created before the Starlark migration framework existed.

### What Gets Created

```
project/
└── migrations/
    ├── morphic.config.yaml   # Tool configuration
    └── .schema_snapshot.yaml        # Empty schema state file
```

### Usage

```bash
morphic init --sql
morphic init --sql --database mysql
```

### Output

```
Created directory: migrations/
Generated: migrations/morphic.config.yaml
Generated: migrations/.schema_snapshot.yaml

Next steps:
  1. Edit schema/schema.star to define your tables
  2. Run: morphic sql-migrations
```

### Generated Configuration File

`migrations/morphic.config.yaml` contains database-appropriate defaults:

```yaml
database:
  type: postgresql         # postgresql, mysql, sqlserver, sqlite
  default_schema: public
  quote_identifiers: true

migration:
  directory: migrations
  file_prefix: "20060102150405"
  snapshot_file: .schema_snapshot.yaml
  auto_apply: false
  include_down_sql: true
  review_comment_prefix: "-- REVIEW: "
  rejection_comment_prefix: "-- REJECTED: "
  silent: false
  destructive_operations:
    - table_removed
    - field_removed
    - index_removed
    - table_renamed
    - field_renamed
    - field_modified

schema:
  search_paths: []
  ignore_modules: []
  schema_file_name: schema.star
  validate_strict: false

output:
  verbose: false
  color_enabled: true
  timestamp_format: "2006-01-02 15:04:05"
```

### Post-Init SQL Workflow

```bash
# After init --sql, define your schema then generate SQL migrations
morphic sql-migrations
```

---

## Error Handling

### Directory Already Exists

```bash
$ morphic init
Error: migrations directory already exists

# Remove it and retry, or supply --sql if you want to re-init the config only
rm -rf migrations/
morphic init
```

### Invalid Database Type

```bash
$ morphic init --database oracle
Error: unsupported database type: oracle

# Supported types:
morphic init --database postgresql
morphic init --database mysql
morphic init --database sqlite
morphic init --database sqlserver
```

### Permission Denied

```bash
$ morphic init
Error: permission denied creating directory: migrations/

# Ensure the current directory is writable, then retry
chmod 755 .
morphic init
```

---

## Best Practices

### Commit Generated Files

All generated files should be committed to version control:

```bash
git add migrations/0001_initial.star   # if generated from snapshot
git commit -m "chore: initialise migration framework"
```

### No Rebuild Step Required

`morphic migrate` reads the latest migration files on every invocation — there is no compile or rebuild step between generating a migration and applying it.

---

## See Also

- [migrate command](./migrate.md) — run `up`, `down`, `status`, `fake` etc.
- [morphic Command](./morphic.md) — Generate a new migration file
- [Configuration Guide](../configuration.md) — Full configuration reference
- [Schema Format Guide](../schema-format.md) — Starlark schema syntax
