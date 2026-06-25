# current-state Command

The `current-state` command reconstructs the schema state from existing migration files and outputs it as YAML. This is the inverse of `schema-to-sql` — instead of showing what the schema would produce, it shows what the migration DAG thinks the current schema looks like.

## Overview

The command walks all existing migration files, builds the migration DAG, applies each operation's `Mutate` in topological order, and serialises the resulting `SchemaState` as YAML to stdout.

This is useful for:

- Debugging why `morphic` keeps generating the same migration
- Verifying the migration chain produces the expected schema
- Comparing the reconstructed state against your `schema/schema.star` files
- Inspecting foreign key, index, and default state after a long chain of migrations

## Usage

```bash
morphic current-state [flags]
```

## Command Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose` | bool | `false` | Show detailed output during DAG loading |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to configuration file |

## Examples

### Basic Usage

```bash
morphic current-state
```

Output (YAML):

```yaml
tables:
    - name: users
      fields:
        - name: id
          type: uuid
          primary_key: true
          nullable: false
          default: new_uuid
        - name: email
          type: varchar
          nullable: false
          length: 255
      indexes:
        - name: idx_users_email
          fields: [email]
          unique: true
defaults:
    postgresql:
        now: CURRENT_TIMESTAMP
        new_uuid: gen_random_uuid()
```

### Comparing Against Schema

Use `diff` to find discrepancies between what the migrations produce and what the schema defines:

```bash
morphic current-state > /tmp/migration_state.yaml
diff /tmp/migration_state.yaml schema/schema.star
```

### Debugging Phantom Migrations

If `morphic generate` keeps generating the same migration (e.g., redundant drop/add foreign key), compare the reconstructed state against your schema to see what field the diff engine thinks has changed:

```bash
morphic current-state | grep -A5 "created_user_id"
```

### Piping to Other Tools

```bash
# Count tables in migration state
morphic current-state | grep "^    - name:" | wc -l

# Extract a specific table
morphic current-state | yq '.tables[] | select(.name == "users")'
```

## How It Works

1. Scans the migrations directory for `.star` files
2. Loads all migration files using the Starlark interpreter
3. Builds the migration DAG and resolves topological order
4. Applies each migration's operations via `Mutate()` to build `SchemaState`
5. Converts `SchemaState` to a YAML `Schema` struct (same conversion used by `morphic generate`)
6. Marshals and prints the YAML to stdout

The conversion in step 5 is the same `schemaStateToYAMLSchema()` function used by the `morphic` command — so the output is exactly what the diff engine sees as the "previous state".

## See Also

- [morphic Command](./morphic.md) — generate migrations from schema changes
- [diff Command](./diff.md) — compare schema against migration state
- [schema-to-sql Command](./schema_to_sql.md) — preview SQL from schema (no migration state)
- [migrate Command](./migrate.md) — apply, rollback, and inspect migrations
