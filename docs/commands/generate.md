# generate Command

The `generate` command is the **primary command** for generating database migrations from Starlark schema definitions. It implements a Django-style migration workflow where each migration is a typed Starlark file registered in a DAG (directed acyclic graph).

## Overview

The `generate` command compares the desired schema (defined in Starlark files) against the current schema (reconstructed by replaying all registered migration files) and generates a new `.star` migration file containing typed operations for each detected change.

Unlike the SQL-mode commands, migrations are `.star` source files. They run via `morphic migrate up` (etc.), which loads them in-process with the Starlark interpreter — no compilation step, no temporary binary, no Go toolchain at runtime.

## Usage

```bash
morphic generate [flags]
```

## Command Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auto-approve` | bool | `false` | Automatically approve all destructive operations without prompting (for CI/non-TTY environments) |
| `--check` | bool | `false` | Exit with error code 1 if migrations are needed (CI/CD mode) |
| `--dry-run` | bool | `false` | Print change summary and annotated migration source without writing a file; exits with code 1 if destructive operations are detected |
| `--json` | bool | `false` | Output dry-run results as structured JSON (requires `--dry-run`) |
| `--merge` | bool | `false` | Generate a merge migration for detected concurrent branches |
| `--name` | string | auto-generated | Custom name suffix for the migration file |
| `--verbose` | bool | `false` | Show detailed pipeline output |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to configuration file |

## How It Works

The command runs a five-step pipeline each time it is invoked.

### Step 1 — Scan for existing migration files

The command scans the `migrations/` directory (as configured) for `*.star` files. If no migration files exist, the current schema state is treated as empty.

### Step 2 — Query the DAG for the current schema state

When migration files exist, the command:

1. Loads all `*.star` files in the migrations directory in-process via `internal/interp.LoadRegistry`. The Starlark interpreter runs each file's top-level `migration()` call, registering its `Migration` into a fresh `*migrate.Registry`.
2. Calls `migrate.BuildGraph(reg).ToDAGOutput()` to produce a `DAGOutput` value containing:
   - The full migration graph (names, dependencies, operations)
   - The reconstructed `SchemaState` (all tables, fields, and indexes after replaying every migration in topological order)
   - The list of leaf migrations (the "tips" of the graph that a new migration must depend on)
   - Whether the graph has branches (concurrent development)

The registry and graph live only for the duration of this query; nothing is written to disk.

### Step 3 — Parse the schema

The command parses `schema/schema.star` (and any files it includes) to produce the **desired** schema state. This uses the same Starlark schema parser as all other `morphic` commands.

### Step 4 — Diff the two schemas

The diff engine compares:
- **Previous state**: the `SchemaState` reconstructed from the DAG (or empty if no migrations exist)
- **Current state**: the desired schema from YAML

Detected changes include table additions, removals, renames, field additions, removals, modifications, renames, and index additions and removals.

### Step 5 — Generate or check

Depending on the flags:
- **`--check`**: If any changes are detected, exit with error code 1. No file is written.
- **`--merge`**: Generate a merge migration (see [Branch and Merge Workflow](#branch-and-merge-workflow)).
- **Default**: Generate a new `.go` migration file in the migrations directory.

## Generated File Format

Each generated file calls the top-level `migration()` function. This ensures the migration is automatically registered when the Starlark interpreter loads the file.

```python
# migrations/0001_initial.star
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        create_table("users",
            fields = [
                uuid("id", primary_key = True),
                string("email", length = 255),
                timestamp("created_at", auto_create = True),
            ],
            indexes = [
                index("idx_users_email", fields = ["email"], unique = True),
            ],
        ),
    ],
)
```

### File Naming Convention

```
migrations/NNNN_name.star
```

Where `NNNN` is a zero-padded four-digit sequence number based on the count of existing migration files, and `name` is either the `--name` flag value (lowercased, spaces replaced with underscores) or a name auto-generated from the diff content.

Examples:
- `migrations/0001_initial.star`
- `migrations/0002_add_products.star`
- `migrations/0003_rename_user_email.star`
- `migrations/0004_merge.star` (merge migration)

## Operation Types

There are 10 typed operation types. Each operation implements `Up()` (forward SQL), `Down()` (reverse SQL), and `Mutate()` (updates the in-memory schema state for DAG traversal).

### create_table

Creates a new database table with the specified fields and indexes.

```python
create_table("products",
    fields = [
        uuid("id", primary_key = True),
        string("name", length = 255),
        decimal("price", precision = 10, scale = 2),
        boolean("active", default = True),
        timestamp("created_at", auto_create = True),
        timestamp("updated_at", auto_update = True),
    ],
    indexes = [
        index("idx_products_name", fields = ["name"], unique = False),
    ],
)
```

- **Destructive**: No
- **Down**: emits `DROP TABLE`

### drop_table

Drops an existing database table.

```python
drop_table("old_sessions")
```

- **Destructive**: Yes — all data in the table is lost
- **Down**: reconstructs `CREATE TABLE` from the pre-drop schema state

### rename_table

Renames an existing table.

```python
rename_table("users", "accounts")
```

- **Destructive**: No
- **Down**: emits the reverse rename

### add_field

Adds a new column to an existing table.

```python
add_field("users", string("phone", length = 20, nullable = True))
```

- **Destructive**: No
- **Down**: emits `DROP COLUMN`

### drop_field

Removes a column from an existing table.

```python
drop_field("users", "legacy_token")
```

- **Destructive**: Yes — all data in that column is lost
- **Down**: reconstructs `ADD COLUMN` from the pre-drop schema state

### alter_field

Changes a column's type, length, nullability, default, or other constraints. Both the old and new field definitions are stored so the operation can be reversed exactly.

```python
alter_field("users",
    old_field = string("status", length = 50, nullable = True),
    new_field = string("status", length = 100, nullable = True),
)
```

- **Destructive**: No (though incompatible type changes may fail at the database level)
- **Down**: emits the reverse `ALTER COLUMN` restoring the old definition

### rename_field

Renames a column in an existing table.

```python
rename_field("users", "username", "display_name")
```

- **Destructive**: No
- **Down**: emits the reverse rename

### add_index

Creates an index on one or more columns of an existing table.

```python
add_index("orders", index("idx_orders_user_id", fields = ["user_id", "created_at"], unique = False))
```

- **Destructive**: No
- **Down**: emits `DROP INDEX`

### drop_index

Drops an index from a table.

```python
drop_index("orders", "idx_orders_legacy")
```

- **Destructive**: No (index can be recreated)
- **Down**: reconstructs `CREATE INDEX` from the pre-drop schema state

### run_sql

Executes raw SQL directly. Used for data migrations, custom constraints, triggers, or any operation that cannot be expressed as a typed operation. `run_sql` does not update the schema state.

```python
run_sql(
    forward = "UPDATE users SET status = 'active' WHERE status IS NULL;",
    backward = "UPDATE users SET status = NULL WHERE status = 'active';",
)
```

- **Destructive**: No (depends entirely on the SQL content)
- **Down**: executes `backward` SQL
- **Note**: `run_sql` operations are not auto-generated by the diff engine. Add them manually when needed.

## Destructive Operation Prompt

When the diff engine detects a destructive change (e.g. `DropTable`, `DropField`), morphic pauses and prompts for a decision before generating the migration:

```
⚠  Destructive operation detected: table_removed on "ocom_reset_password"
  1) Generate  — include operation in migration
  2) Review    — include with // REVIEW comment
  3) Omit      — skip operation; schema state still advances (SchemaOnly)
  4) Exit      — cancel migration generation
  5) All       — generate all remaining destructive ops without prompting
Choice [1-5]:
```

### Options

| Option | Effect | Generated code |
|--------|--------|----------------|
| **1) Generate** | Operation is included and will run normally on `migrate up` | `drop_table("...")` |
| **2) Review** | Operation is included but preceded by a `# REVIEW` comment to flag for human inspection | `# REVIEW\ndrop_table("...")` |
| **3) Omit** | Operation is included with `schema_only = True` — schema state advances but no SQL is executed | `drop_table("...", schema_only = True)` |
| **4) Exit** | Migration generation is cancelled; no file is written | — |
| **5) All** | Remaining destructive operations all use option 1 without further prompting | — |

### schema_only

When `schema_only = True` is set on an operation, the runner treats the table or field as already removed from the database (no SQL is executed) but updates the in-memory schema state as if it had been. This is useful when you have already manually dropped the table or field outside of migrations.

### Skipping the Prompt

Use `--silent` to auto-accept all destructive operations as **Generate** without prompting:

```bash
morphic generate --silent
```

This is equivalent to always choosing option 1. Useful in automated or non-interactive environments.

## Field Type Reference

Starlark schema files use typed field builtins. The following builtins are available:

| Builtin | Key Parameters | Description |
|---------|---------------|-------------|
| `uuid(name)` | `primary_key`, `nullable` | UUID column |
| `string(name, length)` | `nullable`, `default` | VARCHAR column |
| `text(name)` | `nullable`, `default` | TEXT column |
| `integer(name)` | `nullable`, `default` | INTEGER column |
| `bigint(name)` | `nullable`, `default` | BIGINT column |
| `boolean(name)` | `nullable`, `default` | BOOLEAN column |
| `timestamp(name)` | `nullable`, `default`, `auto_create`, `auto_update` | TIMESTAMP column |
| `date(name)` | `nullable`, `default` | DATE column |
| `decimal(name)` | `precision`, `scale`, `nullable`, `default` | DECIMAL/NUMERIC column |
| `json(name)` | `nullable` | JSON column |
| `jsonb(name)` | `nullable` | JSONB column |
| `foreign_key(name, table)` | `on_delete`, `nullable` | Foreign key constraint |

### foreign_key

```python
foreign_key("user_id", table = "users", on_delete = "CASCADE", nullable = False)
```

## Examples

### Basic Usage

```bash
# Generate a migration from detected schema changes
morphic generate

# Output (when changes are detected)
Created migrations/0002_add_products.go

# Output (when no changes are detected)
No changes detected.
```

### With a Custom Name

```bash
morphic generate --name "add_products"
# Generates: migrations/0002_add_products.go

morphic generate --name "Add User Preferences"
# Generates: migrations/0003_add_user_preferences.go
```

### Dry Run

Preview what the migration will do without writing a file. The output includes a
change summary with destructive operations flagged, followed by the annotated
migration source:

```bash
morphic generate --dry-run
```

```
Morphic Dry Run: 0002_remove_sessions

Changes (3):

  Tables removed (1):  [DESTRUCTIVE]
    - sessions
  Fields added (1):
    - users.phone
  Fields removed (1):  [DESTRUCTIVE]
    - users.old_email

WARNING: 2 destructive operation(s) detected:
  - Remove table 'sessions'
  - Remove field 'users.old_email'

--- Migration Source ---
migration(
    name = "0002_remove_sessions",
    dependencies = ["0001_initial"],
    operations = [
        # DESTRUCTIVE: Remove table 'sessions'
        drop_table("sessions"),
        add_field("users", varchar("phone", 20, nullable = True)),
        # DESTRUCTIVE: Remove field 'users.old_email'
        drop_field("users", "old_email"),
    ],
)
```

**Exit codes:** `--dry-run` exits with code **0** when no destructive operations
are present, and code **1** when destructive operations are detected. This
allows CI pipelines to gate on destructive changes without parsing the output.

### Dry Run with JSON Output

For machine-readable output (useful for AI agents and automation):

```bash
morphic generate --dry-run --json
```

```json
{
  "migration_name": "0002_remove_sessions",
  "dependencies": ["0001_initial"],
  "has_destructive": true,
  "destructive_count": 2,
  "changes": [
    {
      "type": "table_removed",
      "table": "sessions",
      "destructive": true,
      "description": "Remove table 'sessions'"
    },
    {
      "type": "field_added",
      "table": "users",
      "field": "phone",
      "destructive": false,
      "description": "Add field 'users.phone'"
    },
    {
      "type": "field_removed",
      "table": "users",
      "field": "old_email",
      "destructive": true,
      "description": "Remove field 'users.old_email'"
    }
  ],
  "source": "migration(\n    name = \"0002_remove_sessions\",\n    ..."
}
```

The same exit code rules apply: code 1 if `has_destructive` is true.

### CI/CD Check Mode

```bash
morphic generate --check

# Exit codes:
# 0 — schema is up to date with all migrations
# 1 — migrations are needed or an error occurred
```

### Verbose Output

```bash
morphic generate --verbose

# Output
Loading migrations/ via Starlark interpreter...
No changes detected.
```

## Full Example Workflow

### Starting a New Project

```bash
# 1. Initialise the migrations directory
morphic init

# 2. Edit the schema
vim schema/schema.star

# 3. Generate the first migration
morphic generate --name "initial"
# Created migrations/0001_initial.star

# 4. Apply (Starlark interpreter loads the .star file in-process)
morphic migrate up
```

### Adding a New Table

```bash
# 1. Add the 'products' table to schema/schema.star

# 2. Generate the migration
morphic generate --name "add_products"
# Created migrations/0002_add_products.star

# 3. Review the generated file
cat migrations/0002_add_products.star

# 4. Apply
morphic migrate up
```

### Altering an Existing Field

```bash
# 1. Change 'status' field from varchar(50) to varchar(100) in schema/schema.star

# 2. Generate
morphic generate --name "expand_user_status"
# Created migrations/0003_expand_user_status.star

# 3. Apply
morphic migrate up
```

## Branch and Merge Workflow

When two developers generate migrations from the same parent migration concurrently, the DAG gains two leaf nodes — a branching structure. The command detects this automatically.

### Detecting Branches

```bash
morphic generate

# Output when branches are detected
WARNING: Branches detected: 0002_add_products, 0002_add_orders
Run 'morphic generate --merge' to generate a merge migration.
```

### Generating a Merge Migration

A merge migration has two (or more) entries in `Dependencies` and an empty `Operations` list. It unifies the branches into a single leaf so subsequent migrations have one clear parent.

```bash
morphic generate --merge
# Created merge migration: migrations/0003_merge_0002_add_products_and_0002_add_orders.star
# Dependencies: 0002_add_products, 0002_add_orders
```

The generated file looks like:

```python
# migrations/0003_merge_0002_add_products_and_0002_add_orders.star
migration(
    name = "0003_merge_0002_add_products_and_0002_add_orders",
    dependencies = [
        "0002_add_products",
        "0002_add_orders",
    ],
    operations = [],
)
```

After the merge migration is committed, both branches can apply `./migrate up` in any order. The merge node ensures the graph remains acyclic with a single leaf.

### Merge with Dry Run

```bash
morphic generate --merge --dry-run
```

## CI/CD Integration

### GitHub Actions

```yaml
# .github/workflows/check-migrations.yml
name: Check Migrations
on: [push, pull_request]

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Install morphic
        run: go install github.com/ocomsoft/morphic@latest
      - name: Check for pending migrations
        run: morphic generate --check
```

### Shell Script

```bash
#!/bin/bash
# dev-migrate.sh
set -e

echo "Checking for schema changes..."
if morphic generate --check 2>/dev/null; then
    echo "No migrations needed"
else
    echo "Generating migrations..."
    morphic generate --verbose

    echo "Applying migrations..."
    morphic migrate up

    echo "Done"
fi
```

## The Migrations Directory Structure

After initialisation and several generated migrations, the `migrations/` directory looks like:

```
migrations/
├── 0001_initial.star     # Auto-generated
├── 0002_add_products.star
└── 0003_expand_user_status.star
```

## After Generating a Migration

There is **no rebuild step**. `morphic migrate` re-reads the latest migration files on every invocation and interprets them in-process via the Starlark interpreter:

```bash
morphic migrate up
```

To verify the migration was applied:

```bash
morphic migrate status
```

To roll back the last migration:

```bash
morphic migrate down
```

To view the full DAG:

```bash
morphic migrate dag
morphic migrate dag --format json
```

## Configuration Integration

The command reads `migrations/morphic.config.yaml`:

```yaml
database:
  type: postgresql          # Target database: postgresql, mysql, sqlite, sqlserver

migration:
  directory: migrations     # Where .star migration files are written
```

## Error Handling

### Common Errors

**No schema files found**
```
Error: parsing schema: no schema files found
```
Create `schema/schema.star` or check the search paths.

**Starlark load failure in migrations directory**
```
Error: querying migration DAG: loading migrations: interpreting <file>: ...
```
The Starlark interpreter failed to load a migration file. Common causes:
- A typo or syntax error in a hand-edited migration. Open the file in your editor and check the reported line number.
- An unsupported Starlark expression or built-in. Refer to the [Schema Format Guide](../schema-format.md) for the list of available builtins.

**Missing dependency**
```
Error: querying migration DAG: migration "0003_add_orders" depends on "0002_missing" which is not registered
```
A migration file references a dependency that does not exist. Check the `Dependencies` field in the affected migration file.

**Branches detected without --merge**
```
WARNING: Branches detected: 0002_add_products, 0002_add_orders
Run 'morphic generate --merge' to generate a merge migration.
```
Run with `--merge` to resolve.

**Check mode failure**
```
Error: migrations needed: 3 changes detected
```
Exit code 1. Schema and migrations are out of sync. Generate the migration and commit it.

## See Also

- [init Command](./init.md) — Initialise the migrations directory
- [Schema Format Guide](../schema-format.md) — Complete Starlark schema reference
- [Configuration Guide](../configuration.md) — Configuration options
- [Architecture Guide](../architecture.md) — How the DAG and migration framework work
