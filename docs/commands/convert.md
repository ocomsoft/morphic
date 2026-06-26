# from-makemigrations Command

The `from-makemigrations` command converts existing legacy migration files to Starlark (`.star`) format. This is a one-time migration tool for projects adopting the Starlark migration format from the old makemigrations format.

## Overview

The command loads each legacy `.go` migration file, extracts its operations (create table, add field, alter field, etc.), and emits an equivalent `.star` file using the Starlark DSL. The conversion is lossless — the resulting Starlark migrations produce identical database operations.

## Usage

```bash
morphic from-makemigrations <migrations-dir> -o <output-dir>
```

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `migrations-dir` | Yes | Path to the directory containing legacy `.go` migration files |

## Command Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output, -o` | string | (required) | Output directory for `.star` files |

## Examples

Convert all legacy migrations to Starlark:

```bash
morphic from-makemigrations migrations/ -o migrations_starlark/
```

Convert and overwrite in place (replace legacy format with Starlark):

```bash
morphic from-makemigrations migrations/ -o migrations_new/
# Review the output, then:
rm migrations/*.go
mv migrations_new/*.star migrations/
```

## How It Works

1. **Load** — all `.go` files in the migrations directory are loaded and each `Migration` is registered into a registry.
2. **Convert** — each `*migrate.Migration` is serialized to Starlark syntax using typed field builtins (`uuid()`, `varchar()`, `timestamp()`, etc.) and positional arguments for concise output.
3. **Write** — each `.star` file is written to the output directory, named `<migration-name>.star`.

The command preserves:
- Migration names and dependencies
- All operation types (create_table, add_field, alter_field, run_sql, upsert_data, etc.)
- Field types, defaults, nullability, foreign keys, and indexes
- set_defaults and set_type_mappings operations

## Output

```
Loading legacy migrations from migrations/...
Found 26 migration(s). Converting...
  ✓ 0001_initial.star
  ✓ 0002_add_contact.star
  ✓ 0003_create_job.star
  ...

Converted 26 migration(s) to migrations_starlark/
```

## Related

- [Starlark Migration Format](../starlark-migrations.md) — full reference for the `.star` format
- [generate Command](./generate.md) — generate new migrations from schema changes
- [yaml2dsl Command](./yaml2dsl.md) — convert YAML schema files to Starlark DSL
