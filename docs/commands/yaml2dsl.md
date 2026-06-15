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

## Related

- [Starlark Migration Format](../starlark-migrations.md) — full reference for the `.star` format
- [Schema Format](../schema-format.md) — YAML schema file reference
- [convert Command](./convert.md) — convert Go migration files to Starlark
