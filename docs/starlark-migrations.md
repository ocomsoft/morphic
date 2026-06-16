# Starlark Migration Format

Starlark (`.star`) is an alternative migration format alongside Go (`.go`). It uses a Python-like DSL powered by [Starlark-Go](https://pkg.go.dev/go.starlark.net) that is more concise than the Go format while producing identical migration operations at runtime.

Migration files are loaded by `morphic migrate` from your migrations directory. Each `.star` file defines exactly one migration using the `migration()` function.

> **When to use Starlark vs Go.** Starlark is ideal for schema changes, data seeding, and most day-to-day migrations. Use Go (`.go`) migrations when you need to import third-party packages, use complex control flow, or call Go APIs that aren't exposed as Starlark builtins.

---

## Quick Start

```starlark
migration(
    name = "0001_initial",
    operations = [
        set_defaults({"new_uuid": "gen_random_uuid()", "now": "CURRENT_TIMESTAMP"}),
        create_table("contact",
            fields = [
                uuid("id", primary_key=True, default="new_uuid"),
                varchar("email", 255),
                varchar("name", 100, nullable=True),
                timestamp("created_date", default="now"),
                timestamp("modified_date", nullable=True),
            ],
            indexes = [
                index("contact_email_idx", ["email"], unique=True),
            ],
        ),
    ],
)
```

Save as `migrations/0001_initial.star` and run `morphic migrate`.

---

## Migration Structure

Every `.star` file must call `migration()` exactly once at the top level.

```starlark
migration(
    name = "0005_add_job_notes",
    dependencies = ["0004_add_item_category"],
    operations = [
        # operations go here
    ],
)
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Migration name, must match filename (without `.star`) |
| `dependencies` | list of strings | No | Migration names this depends on (default: `[]`) |
| `operations` | list of operations | No | Operations to apply (default: `[]`) |

Dependencies form a directed acyclic graph (DAG). A migration can depend on multiple parents:

```starlark
migration(
    name = "0008_drop_legacy_table",
    dependencies = ["0006_add_job_indexes", "0007_migrate_job_data"],
    operations = [
        drop_table("legacy_job", ignore_errors=True),
    ],
)
```

---

## Field Types

Fields describe table columns. There are two ways to define a field: **typed builtins** (preferred) and the generic `field()` fallback.

### Typed Field Builtins

Each type has a dedicated function with type-appropriate positional arguments:

| Builtin | Positional Args | Example |
|---------|----------------|---------|
| `uuid(name)` | name | `uuid("id", primary_key=True, default="new_uuid")` |
| `varchar(name, length)` | name, length | `varchar("email", 255)` |
| `text(name)` | name | `text("notes", nullable=True)` |
| `integer(name)` | name | `integer("sort_order", default="zero")` |
| `bigint(name)` | name | `bigint("total_bytes")` |
| `boolean(name)` | name | `boolean("is_active", default="true")` |
| `timestamp(name)` | name | `timestamp("created_date", default="now")` |
| `date(name)` | name | `date("due_date", nullable=True)` |
| `time(name)` | name | `time("start_time")` |
| `float(name)` | name | `float("weight", nullable=True)` |
| `decimal(name, precision, scale)` | name, precision, scale | `decimal("unit_price", 10, 2)` |
| `jsonb(name)` | name | `jsonb("metadata", default="object")` |
| `bytes(name)` | name | `bytes("file_data")` |
| `serial(name)` | name | `serial("seq_no")` |
| `foreign_key(name, fk)` | name, fk dict | `foreign_key("contact_id", fk("contact", on_delete="CASCADE"))` |

All typed builtins accept these common keyword arguments:

| Keyword | Type | Default | Description |
|---------|------|---------|-------------|
| `nullable` | bool | `False` | Allow NULL values |
| `primary_key` | bool | `False` | Mark as primary key |
| `default` | string | `""` | Symbolic default name (resolved at runtime via `set_defaults`) |

Datetime types (`timestamp`, `date`, `time`) also accept:

| Keyword | Type | Default | Description |
|---------|------|---------|-------------|
| `auto_create` | bool | `False` | Auto-set on row creation (e.g. `created_at`) |
| `auto_update` | bool | `False` | Auto-set on row update (e.g. `updated_at`) |

These are only valid on datetime fields — passing them to other types (e.g. `varchar`, `integer`) will raise an error.

### Generic Field Fallback

For column types without a dedicated builtin, use `field()`:

```starlark
field("name", "citext", nullable=True)
field("data", "hstore", default="blank")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `name` | 1st | string | Yes | Column name |
| `type` | 2nd | string | Yes | SQL type name |
| `nullable` | keyword | bool | No | Allow NULL (default: `False`) |
| `primary_key` | keyword | bool | No | Primary key (default: `False`) |
| `default` | keyword | string | No | Symbolic default name |
| `length` | keyword | int | No | Max length (default: `0`) |
| `precision` | keyword | int | No | Decimal precision (default: `0`) |
| `scale` | keyword | int | No | Decimal scale (default: `0`) |
| `auto_create` | keyword | bool | No | Auto-set on create (default: `False`) |
| `auto_update` | keyword | bool | No | Auto-set on update (default: `False`) |
| `foreign_key` | keyword | dict | No | FK reference from `fk()` |
| `many_to_many` | keyword | string | No | M2M target table name |

### Foreign Key References

The `fk()` helper builds a foreign key reference dict:

```starlark
fk("contact", on_delete="CASCADE")
fk("item", on_delete="SET_NULL", on_update="CASCADE")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Referenced table name |
| `on_delete` | keyword | string | No | Delete action: `CASCADE`, `PROTECT`, `SET_NULL`, `SET_DEFAULT`, `NO_ACTION`, `RESTRICT` |
| `on_update` | keyword | string | No | Update action (same options as `on_delete`) |

Use `fk()` with the `foreign_key` typed builtin:

```starlark
foreign_key("created_by_id", fk("users", on_delete="PROTECT"), nullable=True)
```

Or with the generic `field()`:

```starlark
field("owner_id", "uuid", foreign_key=fk("users", on_delete="CASCADE"), nullable=True)
```

### Indexes

The `index()` helper defines table indexes:

```starlark
index("contact_email_idx", ["email"], unique=True)
index("job_contact_date_idx", ["contact_id", "due_date"])
index("item_metadata_idx", ["metadata"], method="GIN")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `name` | 1st | string | Yes | Index name |
| `fields` | 2nd | list of strings | Yes | Column names to index |
| `unique` | keyword | bool | No | Unique constraint (default: `False`) |
| `method` | keyword | string | No | Index method: `BTREE`, `HASH`, `GIN`, `GiST`, `BRIN` |
| `where` | keyword | string | No | Partial index WHERE clause |

---

## Operations Reference

### create_table

Creates a new table with fields and optional indexes.

```starlark
create_table("job",
    fields = [
        uuid("id", nullable=True, primary_key=True, default="new_uuid"),
        varchar("title", 255),
        text("description", nullable=True),
        varchar("status", 20, default="blank"),
        decimal("budget", 10, 2, nullable=True),
        date("due_date", nullable=True),
        timestamp("created_date", default="now"),
        timestamp("modified_date", nullable=True),
        foreign_key("contact_id", fk("contact", on_delete="CASCADE")),
        foreign_key("created_by_id", fk("users", on_delete="PROTECT"), nullable=True),
    ],
    indexes = [
        index("job_contact_idx", ["contact_id"]),
        index("job_status_idx", ["status"]),
        index("job_due_date_idx", ["due_date"]),
    ],
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `name` | 1st | string | Yes | Table name |
| `fields` | keyword | list of field dicts | No | Column definitions |
| `indexes` | keyword | list of index dicts | No | Index definitions |
| `schema_only` | keyword | bool | No | Track in schema without executing DDL (default: `False`) |

### drop_table

Drops a table.

```starlark
drop_table("legacy_job", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `name` | 1st | string | Yes | Table name to drop |
| `schema_only` | keyword | bool | No | Schema-only removal (default: `False`) |
| `ignore_errors` | keyword | bool | No | Suppress errors if table doesn't exist (default: `False`) |

### rename_table

Renames a table.

```starlark
rename_table("item", "product")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `old_name` | 1st | string | Yes | Current table name |
| `new_name` | 2nd | string | Yes | New table name |

### add_field

Adds a column to an existing table.

```starlark
add_field("contact", varchar("phone", 20, nullable=True))
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `field` | 2nd | field dict | Yes | Field definition from any field builtin |
| `schema_only` | keyword | bool | No | Schema-only (default: `False`) |

### drop_field

Removes a column from a table.

```starlark
drop_field("contact", "fax_number")
drop_field("item", "legacy_code", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `field_name` | 2nd | string | Yes | Column name to drop |
| `schema_only` | keyword | bool | No | Schema-only (default: `False`) |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### alter_field

Changes a column's type, length, nullability, or default. Requires both old and new field definitions so the migration can be reversed.

```starlark
alter_field("parts",
    old_field = varchar("part_no", 16, nullable=True),
    new_field = varchar("part_no", 40, nullable=True),
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `old_field` | keyword | field dict | Yes | Current field definition |
| `new_field` | keyword | field dict | Yes | New field definition |

### rename_field

Renames a column.

```starlark
rename_field("contact", "fname", "first_name")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `old_name` | 2nd | string | Yes | Current column name |
| `new_name` | 3rd | string | Yes | New column name |

### add_index

Adds an index to an existing table.

```starlark
add_index("contact",
    index("contact_email_idx", ["email"], unique=True),
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `index` | 2nd | index dict | Yes | Index definition from `index()` |

### drop_index

Removes an index.

```starlark
drop_index("contact", "contact_email_idx")
drop_index("job", "job_legacy_idx", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `index_name` | 2nd | string | Yes | Index name to drop |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### add_foreign_key

Adds a foreign key constraint to an existing column. This is separate from defining a `foreign_key` field in `create_table` — use this when the FK constraint needs to be added after table creation (e.g., circular references) or when adding a constraint to an existing table.

```starlark
add_foreign_key("job", "contact_id", "fk_job_contact_id", "contact",
    on_delete = "CASCADE",
    ignore_errors = True,
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table containing the FK column |
| `field_name` | 2nd | string | Yes | Column name |
| `constraint_name` | 3rd | string | Yes | FK constraint name |
| `referenced_table` | 4th | string | Yes | Table being referenced |
| `on_delete` | keyword | string | No | Delete action (default: `"CASCADE"`) |
| `on_update` | keyword | string | No | Update action (default: `""`) |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### drop_foreign_key

Removes a foreign key constraint.

```starlark
drop_foreign_key("job", "fk_job_category_id", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table containing the FK |
| `constraint_name` | 2nd | string | Yes | FK constraint name to drop |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### run_sql

Executes arbitrary SQL. Use for operations that don't have a dedicated builtin (extensions, data backfills, complex DDL).

```starlark
# Data backfill
run_sql(
    forward = "UPDATE item SET display_name = COALESCE(NULLIF(TRIM(title), ''), NULLIF(TRIM(sku), ''))",
)

# Forward and backward SQL
run_sql(
    forward = "CREATE EXTENSION IF NOT EXISTS pgcrypto",
    backward = "DROP EXTENSION IF EXISTS pgcrypto",
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `forward` | keyword | string | Yes | SQL to run on migrate (up) |
| `backward` | keyword | string | No | SQL to run on rollback (down) (default: `""`) |

For multi-line SQL, use triple-quoted strings:

```starlark
run_sql(
    forward = """
        CREATE OR REPLACE FUNCTION update_modified_date()
        RETURNS TRIGGER AS $$
        BEGIN
            NEW.modified_date = CURRENT_TIMESTAMP;
            RETURN NEW;
        END;
        $$ LANGUAGE plpgsql
    """,
    backward = "DROP FUNCTION IF EXISTS update_modified_date()",
)
```

### upsert_data

Inserts or updates rows in a table. Uses `INSERT ... ON CONFLICT ... DO UPDATE` under the hood.

```starlark
upsert_data("job_status",
    conflict_keys = ["code"],
    rows = [
        row(code="DRAFT", description="Draft", sort_order=1),
        row(code="OPEN", description="Open", sort_order=2),
        row(code="CLOSED", description="Closed", sort_order=3),
    ],
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `conflict_keys` | keyword | list of strings | Yes | Columns forming the conflict/upsert key |
| `rows` | keyword | list of row dicts | Yes | Data rows from `row()` |

The `row()` helper takes keyword-only arguments — each keyword becomes a column name:

```starlark
row(id="abc-123", name="Widget", price=19.99, description=None)
```

Values can be strings, integers, floats, booleans, or `None` (maps to SQL `NULL`).

**Note:** `rows` and `file` are mutually exclusive — use one or the other.

### File-backed upsert data

For large datasets (country codes, postal data, product catalogs), you can store row data in a JSONL file instead of embedding it inline:

```starlark
upsert_data("countries",
    conflict_keys = ["code"],
    file = "data/countries.jsonl",
)
```

The `file` parameter is a path relative to the migrations directory. The JSONL file contains one JSON object per line:

```
{"code":"AU","name":"Australia","population":25687041}
{"code":"US","name":"United States","population":331002651}
{"code":"NZ","name":"New Zealand","population":5084300}
```

JSONL files support:
- Empty lines (skipped)
- Comment lines starting with `//` (skipped)
- All objects must have the same set of keys

The `file` parameter for `upsert_data` accepts these additional parameters when used in place of `rows`:

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `conflict_keys` | keyword | list of strings | Yes | Columns forming the conflict/upsert key |
| `file` | keyword | string | Yes | Path to JSONL file, relative to the migrations directory |

Using `file=` keeps migration files concise and allows large reference datasets to be reviewed and diff'd independently of the migration logic. This is the recommended approach for any dataset with more than a handful of rows.

### set_defaults

Registers symbolic default names that field `default=` values resolve to at runtime. Must appear before any operation that uses the symbolic defaults.

```starlark
set_defaults({
    "new_uuid": "gen_random_uuid()",
    "now": "CURRENT_TIMESTAMP",
    "today": "CURRENT_DATE",
    "zero": "0",
    "false": "false",
    "true": "true",
    "blank": "''",
    "null": "null",
    "object": "'{}'::jsonb",
    "array": "'[]'::jsonb",
})
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `mapping` | 1st | dict | Yes | Map of symbolic name to SQL expression |

When a field specifies `default="new_uuid"`, the runtime resolves it to `gen_random_uuid()` (or whatever the current defaults map contains). This allows the same migration to work across different database engines by changing only the defaults.

### set_type_mappings

Overrides SQL type names for the current database provider. Useful when a migration targets a specific database dialect.

```starlark
set_type_mappings({"float": "DOUBLE PRECISION"})
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `mapping` | 1st | dict | Yes | Map of abstract type name to provider-specific SQL type |

---

## Common Patterns

### Initial Migration with Defaults

The first migration typically sets up defaults and type mappings before creating tables:

```starlark
migration(
    name = "0001_initial",
    operations = [
        set_type_mappings({"float": "DOUBLE PRECISION"}),
        set_defaults({
            "new_uuid": "gen_random_uuid()",
            "now": "CURRENT_TIMESTAMP",
            "today": "CURRENT_DATE",
            "zero": "0",
            "false": "false",
            "true": "true",
        }),
        create_table("contact",
            fields = [
                uuid("id", nullable=True, primary_key=True, default="new_uuid"),
                varchar("email", 255),
                varchar("first_name", 100),
                varchar("last_name", 100),
                timestamp("created_date", default="now"),
            ],
            indexes = [
                index("contact_email_idx", ["email"], unique=True),
            ],
        ),
    ],
)
```

### Table with Foreign Keys and Deferred Constraints

When a table has foreign keys, define the column with `foreign_key()` in `create_table`, then add the named constraint separately:

```starlark
migration(
    name = "0003_create_job",
    dependencies = ["0002_create_parts"],
    operations = [
        create_table("job",
            fields = [
                uuid("id", nullable=True, primary_key=True, default="new_uuid"),
                varchar("title", 120),
                text("description", nullable=True),
                integer("priority", default="zero"),
                timestamp("created_date", default="now"),
                timestamp("modified_date", nullable=True),
                foreign_key("contact_id", fk("contact", on_delete="CASCADE")),
                foreign_key("created_by_id", fk("users", on_delete="PROTECT"), nullable=True),
                foreign_key("modified_by_id", fk("users", on_delete="PROTECT"), nullable=True),
            ],
        ),
        add_foreign_key("job", "created_by_id", "fk_job_created_by_id", "users",
            on_delete = "PROTECT",
            ignore_errors = True,
        ),
        add_foreign_key("job", "modified_by_id", "fk_job_modified_by_id", "users",
            on_delete = "PROTECT",
            ignore_errors = True,
        ),
    ],
)
```

### Adding Fields with Data Backfill

Add a column, then populate it with a `run_sql`:

```starlark
migration(
    name = "0004_add_display_name",
    dependencies = ["0003_create_job"],
    operations = [
        add_field("item", varchar("display_name", 255, nullable=True)),
        run_sql(
            forward = "UPDATE item SET display_name = COALESCE(NULLIF(TRIM(title), ''), NULLIF(TRIM(sku), ''))",
        ),
        add_field("contact", varchar("full_name", 200, nullable=True)),
        run_sql(
            forward = "UPDATE contact SET full_name = TRIM(first_name || ' ' || last_name)",
        ),
    ],
)
```

### Widening a Column

Use `alter_field` with both old and new definitions:

```starlark
migration(
    name = "0005_widen_part_number",
    dependencies = ["0004_add_display_name"],
    operations = [
        alter_field("parts",
            old_field = varchar("part_no", 16, nullable=True),
            new_field = varchar("part_no", 40, nullable=True),
        ),
    ],
)
```

### Seeding Reference Data

Use `upsert_data` with `row()` to seed or update reference data:

```starlark
migration(
    name = "0006_seed_job_status",
    dependencies = ["0005_widen_part_number"],
    operations = [
        upsert_data("job_status",
            conflict_keys = ["id"],
            rows = [
                row(
                    id="a1b2c3d4-0000-0000-0000-000000000001",
                    code="DRAFT",
                    description="Draft - not yet submitted",
                    sort_order=1,
                    created_date="2024-01-01 00:00:00.000000",
                ),
                row(
                    id="a1b2c3d4-0000-0000-0000-000000000002",
                    code="OPEN",
                    description="Open - in progress",
                    sort_order=2,
                    created_date="2024-01-01 00:00:00.000000",
                ),
                row(
                    id="a1b2c3d4-0000-0000-0000-000000000003",
                    code="CLOSED",
                    description="Closed - completed",
                    sort_order=3,
                    created_date="2024-01-01 00:00:00.000000",
                ),
            ],
        ),
    ],
)
```

### Dropping Fields and Constraints

Drop foreign key constraints before dropping the column they reference:

```starlark
migration(
    name = "0007_remove_category",
    dependencies = ["0006_seed_job_status"],
    operations = [
        drop_foreign_key("item", "fk_item_category_id", ignore_errors=True),
        drop_field("item", "category_code"),
        drop_field("item", "category_id"),
    ],
)
```

### Blank Migration Template

`morphic generate --empty` creates a blank template:

```starlark
migration(
    name = "0010_custom",
    dependencies = ["0009_previous"],
    operations = [
        # TODO: Add operations here
    ],
)
```

---

## Converting from Go Migrations

Existing Go (`.go`) migration files can be converted to Starlark format using the `morphic convert` command:

```bash
morphic convert migrations/ -o migrations_starlark/
```

This loads each Go migration via the Yaegi interpreter, extracts the operations, and emits equivalent `.star` files. The conversion is lossless — the resulting Starlark migrations produce identical database operations.

---

## File Naming

Migration files use the pattern `NNNN_description.star` where `NNNN` is a zero-padded sequence number. The `name` parameter inside the file must match the filename (without the `.star` extension):

```
migrations/
    0001_initial.star
    0002_create_parts.star
    0003_create_job.star
```

Files are loaded in alphabetical order, but execution order is determined by the dependency DAG, not filename sort order.
