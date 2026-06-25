# `db-diff` Command

Compares the live database schema against the expected schema state reconstructed
from the migration DAG.

## What It Does

1. **Reads migration files** from the migrations directory by loading them
   in-process via the Starlark interpreter, then reconstructs the "expected"
   schema state by replaying the migration DAG (the same path used by
   `migrate dag --format json`).
2. **Connects to the live database** and extracts the actual schema using the
   provider's `GetDatabaseSchema`.
3. **Normalizes SQL-native type names** (e.g. `character varying` to `varchar`)
   then diffs the two schemas and reports differences in three categories.

## Difference Categories

- **Missing from DB** -- Tables or fields the migrations expect but that are
  absent from the live database. Usually indicates unapplied migrations.
- **Extra in DB** -- Tables or fields in the live database not tracked by any
  migration. Usually indicates manual DDL.
- **Field Differences** -- Columns that exist in both schemas but differ in
  type, length, nullability, or other properties.

## Flags

| Flag         | Default      | Description                                                      |
|--------------|--------------|------------------------------------------------------------------|
| `--host`     | `""`         | Database host (default: localhost)                               |
| `--port`     | `0`          | Database port                                                    |
| `--database` | `""`         | Database name                                                    |
| `--username` | `""`         | Database username                                                |
| `--password` | `""`         | Database password                                                |
| `--sslmode`  | `""`         | SSL mode (default: disable)                                      |
| `--db-type`  | `postgresql` | Database type (`postgresql`, `mysql`, `sqlserver`, `sqlite`)     |
| `--format`   | `text`       | Output format: `text` or `json`                                  |
| `--verbose`  | `false`      | Show detailed processing information                             |

Connection flags can also be supplied via the config file
(`migrations/morphic.config.yaml`). Command-line flags take precedence
over config file settings.

## Exit Codes

| Code | Meaning                                            |
|------|----------------------------------------------------|
| `0`  | No differences -- live DB matches the migration DAG |
| `1`  | Differences found, or an error occurred             |

Exit code `1` makes this command suitable for CI pipelines to detect schema
drift.

## Examples

### Basic local PostgreSQL comparison

```bash
morphic db-diff \
  --host=localhost \
  --port=5432 \
  --database=myapp \
  --username=user \
  --password=secret
```

### Using config file

```bash
morphic db-diff --config=migrations/morphic.config.yaml
```

When the config file already contains the database connection details, no
additional flags are needed.

### JSON output for scripting

```bash
morphic db-diff \
  --host=localhost \
  --database=myapp \
  --format=json | jq '.changes[] | select(.destructive == true)'
```

The JSON output can be piped to `jq` or consumed by other tooling to
programmatically inspect drift.

### CI pipeline usage with environment variables

```bash
morphic db-diff \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --database="$DB_NAME" \
  --username="$DB_USER" \
  --password="$DB_PASS" \
  --format=json

# Exit code 1 fails the pipeline when drift is detected
```

### Verbose output for debugging

```bash
morphic db-diff \
  --host=localhost \
  --database=myapp \
  --verbose
```

Verbose mode prints additional detail for each difference, including the
full change description.

## Type Normalization

Before comparison, SQL-native types returned by database introspection are
normalized to the canonical types used in the schema. This prevents
false-positive diffs caused by databases reporting types differently than how
they were declared in migration files.

The key mappings from `sqlTypeMapping` are:

| SQL-native type                   | Canonical type |
|-----------------------------------|----------------|
| `character varying`               | `varchar`      |
| `character`                       | `varchar`      |
| `char`                            | `varchar`      |
| `int`                             | `integer`      |
| `int2`                            | `integer`      |
| `int4`                            | `integer`      |
| `smallint`                        | `integer`      |
| `int8`                            | `bigint`       |
| `float4`                          | `float`        |
| `float8`                          | `float`        |
| `double precision`                | `float`        |
| `real`                            | `float`        |
| `numeric`                         | `decimal`      |
| `bool`                            | `boolean`      |
| `timestamp without time zone`     | `timestamp`    |
| `timestamp with time zone`        | `timestamp`    |
| `timestamptz`                     | `timestamp`    |
| `serial4`                         | `serial`       |
| `serial8`                         | `serial`       |

Types not in this table are left unchanged.

## Provider Support

`GetDatabaseSchema` is fully implemented for PostgreSQL. Other database types
(MySQL, SQL Server, SQLite) may have partial or placeholder implementations.
Check the `internal/providers/` directory for current support status.
