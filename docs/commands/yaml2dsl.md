# yaml2dsl Command

The `yaml2dsl` command converts a YAML schema file to the Starlark schema DSL format. This is useful when adopting Starlark-based schema definitions alongside or instead of YAML.

## Overview

The command reads a YAML schema file (e.g., `schema.yaml`), parses it into the internal schema representation, and emits an equivalent `.star` file using the Starlark schema DSL.

## Usage

```bash
morphic yaml2dsl <input.yaml> <output.star>
```

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `input.yaml` | Yes | Path to the YAML schema file to convert |
| `output.star` | Yes | Path for the output Starlark schema file |

## Examples

Convert a schema file:

```bash
morphic yaml2dsl migrations/schema.yaml schema.star
```

## Output

```
Reading YAML schema from migrations/schema.yaml...
Wrote Starlark schema to schema.star
```

## Output Format

The command generates a `.star` file using the Starlark schema DSL:

```python
database("app", "1.0.0")

defaults("postgresql", {"blank": "''", "now": "CURRENT_TIMESTAMP"})

table("users",
    fields = [
        serial("id", primary_key=True),
        varchar("name", 255),
        varchar("email", 255, nullable=True),
        timestamp("created_at", auto_create=True),
        foreign_key("org_id", fk("organization", on_delete="CASCADE")),
    ],
    indexes = [
        index("idx_users_email", ["email"], unique=True),
    ],
)
```

## Schema DSL Builtins

### database(name, version)

Declares the database name and version.

```python
database("myapp", "2.0.0")
```

### defaults(db_type, mapping)

Sets default value mappings for a database type.

```python
defaults("postgresql", {
    "blank": "''",
    "now": "CURRENT_TIMESTAMP",
    "today": "CURRENT_DATE",
})
```

### type_mappings(db_type, mapping)

Sets custom SQL type overrides for a database type.

```python
type_mappings("postgresql", {
    "money": "NUMERIC(19,4)",
})
```

### include(module, path)

Includes an external schema module.

```python
include("ocom", "schema/schema.yaml")
```

### table(name, fields=[], indexes=[])

Declares a table with its fields and indexes.

```python
table("orders",
    fields = [
        serial("id", primary_key=True),
        varchar("ref", 50, default="blank"),
        decimal("total", 19, 2),
        foreign_key("customer_id", fk("customer", on_delete="CASCADE")),
    ],
    indexes = [
        index("idx_orders_ref", ["ref"], unique=True),
    ],
)
```

## Available Field Types

| Function | Positional Args | Common kwargs |
|----------|----------------|---------------|
| `serial(name)` | name | `primary_key` |
| `varchar(name, length)` | name, length | `nullable`, `default` |
| `text(name)` | name | `nullable`, `default`, `length` |
| `integer(name)` | name | `nullable`, `default` |
| `bigint(name)` | name | `nullable`, `default` |
| `float(name)` | name | `nullable`, `default` |
| `decimal(name, precision, scale)` | name, precision, scale | `nullable`, `default` |
| `boolean(name)` | name | `nullable`, `default` |
| `date(name)` | name | `nullable`, `default` |
| `timestamp(name)` | name | `nullable`, `default`, `auto_create`, `auto_update` |
| `time(name)` | name | `nullable`, `default` |
| `uuid(name)` | name | `nullable`, `default` |
| `jsonb(name)` | name | `nullable`, `default` |
| `foreign_key(name, fk_ref)` | name, `fk(table, on_delete=...)` | `nullable` |

## Dual Format Support

The schema loader (`interp.LoadSchema`) auto-detects the schema format:

1. `schema.star` exists — loads via Starlark interpreter
2. `schema.yaml` exists — loads via YAML parser
3. Both exist — returns an error
4. Neither exists — returns an error

This means you can convert to `.star` and delete the `.yaml` file, and all existing commands continue to work.

## Related

- [Starlark Migration Format](../starlark-migrations.md) — full reference for the `.star` format
- [Schema Format](../schema-format.md) — YAML schema file reference
- [convert Command](./convert.md) — convert Go migration files to Starlark
