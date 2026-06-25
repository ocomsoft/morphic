# `schema-diff` Command

Compares the current schema files against the migration DAG state and
shows the differences in both directions.

## What It Does

1. **Reads migration files** from the migrations directory by loading them
   in-process via the Starlark interpreter, then reconstructs the "expected"
   schema state by replaying the migration DAG.
2. **Scans and merges schema files** from the current module and its
   dependencies (same discovery used by `morphic generate`).
3. **Diffs the two schemas** and reports differences grouped by direction
   and category.

## Difference Categories

- **In Schema, Not Yet Migrated** — Tables, fields, or indexes present in
  `schema.star` but not yet captured in a migration. Running
  `morphic generate` would generate code for these.
- **In Migrations, Removed from Schema** — Tables, fields, or indexes that
  exist in the migration DAG but have been removed from `schema.star`.
  Running `morphic generate` would generate a drop for these.
- **Field Differences** — Fields that exist in both but differ in added vs
  removed status across tables.
- **Modified Fields** — Fields whose properties (type, length, nullable, etc.)
  changed between migration state and schema.
- **Index Differences** — Indexes added to or removed from schema.
- **Foreign Key Differences** — Foreign key relationships added or removed.

## Output Formats

### Default (color-coded report)

The default output is a human-readable, color-coded report with:

- Bidirectional change listing grouped by category
- A summary with counts per category
- Copy-pasteable YAML snippets for each changed table/field

### YAML (`--yaml`)

Machine-readable YAML with `add`, `remove`, and `modify` sections:

```yaml
add:
  tables:
    - name: new_table
      fields: [...]
  fields:
    - table: existing_table
      field: {name: new_field, type: varchar, ...}
remove:
  tables:
    - name: dropped_table
      fields: [...]
modify:
  - table: changed_table
    field: changed_field
    from: {type: varchar, length: 100}
    to: {type: varchar, length: 255}
```

### JSON (`--json`)

Raw `SchemaDiff` structure as JSON, suitable for piping to `jq` or other tools.

## Flags

| Flag        | Default | Description                                        |
|-------------|---------|---------------------------------------------------|
| `--verbose` | `false` | Show detailed processing and change descriptions   |
| `--yaml`    | `false` | Output as YAML (add/remove/modify sections)        |
| `--json`    | `false` | Output as JSON                                     |

## Examples

### Quick overview of pending changes

```bash
morphic schema-diff
```

### YAML output for scripting

```bash
morphic schema-diff --yaml
```

### JSON output piped to jq

```bash
morphic schema-diff --json | jq '.changes[] | select(.type == "table_removed")'
```

### Verbose mode

```bash
morphic schema-diff --verbose
```

Shows additional detail for each change, including full descriptions.

## Comparison with Other Commands

| Command | Compares | Use Case |
|---------|----------|----------|
| `schema-diff` | schema ↔ migration DAG | See what a migration would do |
| `db-diff` | migration DAG ↔ live database | Detect drift after deployment |
| `morphic --check` | schema ↔ migration DAG | CI gate (exit code only) |
