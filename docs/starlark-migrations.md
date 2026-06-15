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
        create_table("users",
            fields = [
                uuid("id", primary_key=True, default="new_uuid"),
                varchar("email", 255),
                varchar("name", 100, nullable=True),
                timestamp("created_date", default="now"),
                timestamp("modified_date", nullable=True),
            ],
            indexes = [
                index("users_email_idx", ["email"], unique=True),
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
    name = "0025_add_standard_tool",
    dependencies = ["0024_add_image_to_tmc_tml"],
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
    name = "0023_drop_tma_entry",
    dependencies = ["0020_add_missing_view_indexes", "0022_create_missing_indexes"],
    operations = [
        drop_table("tma_entry", ignore_errors=True),
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
| `text(name)` | name | `text("description", nullable=True)` |
| `integer(name)` | name | `integer("display_order", default="zero")` |
| `bigint(name)` | name | `bigint("total_bytes")` |
| `boolean(name)` | name | `boolean("is_active", default="true")` |
| `timestamp(name)` | name | `timestamp("created_date", default="now")` |
| `date(name)` | name | `date("birth_date", nullable=True)` |
| `time(name)` | name | `time("start_time")` |
| `float(name)` | name | `float("temperature", nullable=True)` |
| `decimal(name, precision, scale)` | name, precision, scale | `decimal("price", 10, 2)` |
| `jsonb(name)` | name | `jsonb("metadata", default="object")` |
| `bytes(name)` | name | `bytes("file_data")` |
| `serial(name)` | name | `serial("seq_no")` |
| `foreign_key(name, fk)` | name, fk dict | `foreign_key("user_id", fk("auth_user", on_delete="CASCADE"))` |

All typed builtins accept these keyword arguments:

| Keyword | Type | Default | Description |
|---------|------|---------|-------------|
| `nullable` | bool | `False` | Allow NULL values |
| `primary_key` | bool | `False` | Mark as primary key |
| `default` | string | `""` | Symbolic default name (resolved at runtime via `set_defaults`) |
| `auto_create` | bool | `False` | Auto-set on row creation |
| `auto_update` | bool | `False` | Auto-set on row update |

`decimal()` and `foreign_key()` do not accept `auto_create` or `auto_update`.

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
fk("auth_user", on_delete="CASCADE")
fk("products", on_delete="SET_NULL", on_update="CASCADE")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Referenced table name |
| `on_delete` | keyword | string | No | Delete action: `CASCADE`, `PROTECT`, `SET_NULL`, `SET_DEFAULT`, `NO_ACTION`, `RESTRICT` |
| `on_update` | keyword | string | No | Update action (same options as `on_delete`) |

Use `fk()` with the `foreign_key` typed builtin:

```starlark
foreign_key("created_user_id", fk("auth_user", on_delete="PROTECT"), nullable=True)
```

Or with the generic `field()`:

```starlark
field("owner_id", "uuid", foreign_key=fk("auth_user", on_delete="CASCADE"), nullable=True)
```

### Indexes

The `index()` helper defines table indexes:

```starlark
index("users_email_idx", ["email"], unique=True)
index("audit_table_record_idx", ["table_name", "record_id"])
index("data_gin_idx", ["metadata"], method="GIN")
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
create_table("audit_log",
    fields = [
        uuid("id", nullable=True, primary_key=True, default="new_uuid"),
        varchar("table_name", 255),
        uuid("record_id"),
        varchar("action", 50),
        timestamp("changed_at", default="now"),
        foreign_key("changed_by_id", fk("auth_user", on_delete="PROTECT"), nullable=True),
        jsonb("before_data", nullable=True),
        jsonb("after_data", nullable=True),
    ],
    indexes = [
        index("audit_log_table_record_idx", ["table_name", "record_id"]),
        index("audit_log_changed_at_idx", ["changed_at"]),
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
drop_table("tma_entry", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `name` | 1st | string | Yes | Table name to drop |
| `schema_only` | keyword | bool | No | Schema-only removal (default: `False`) |
| `ignore_errors` | keyword | bool | No | Suppress errors if table doesn't exist (default: `False`) |

### rename_table

Renames a table.

```starlark
rename_table("old_users", "users")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `old_name` | 1st | string | Yes | Current table name |
| `new_name` | 2nd | string | Yes | New table name |

### add_field

Adds a column to an existing table.

```starlark
add_field("wind_tunnel_import_header", varchar("app_version", 50, nullable=True))
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `field` | 2nd | field dict | Yes | Field definition from any field builtin |
| `schema_only` | keyword | bool | No | Schema-only (default: `False`) |

### drop_field

Removes a column from a table.

```starlark
drop_field("stock_lists", "category_code")
drop_field("stock_lists", "category_id", ignore_errors=True)
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
alter_field("stock_lists",
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
rename_field("users", "fname", "first_name")
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `old_name` | 2nd | string | Yes | Current column name |
| `new_name` | 3rd | string | Yes | New column name |

### add_index

Adds an index to an existing table.

```starlark
add_index("users",
    index("users_email_idx", ["email"], unique=True),
)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `index` | 2nd | index dict | Yes | Index definition from `index()` |

### drop_index

Removes an index.

```starlark
drop_index("users", "users_email_idx")
drop_index("orders", "orders_legacy_idx", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table name |
| `index_name` | 2nd | string | Yes | Index name to drop |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### add_foreign_key

Adds a foreign key constraint to an existing column. This is separate from defining a `foreign_key` field in `create_table` — use this when the FK constraint needs to be added after table creation (e.g., circular references) or when adding a constraint to an existing table.

```starlark
add_foreign_key("standard_tool", "created_user_id", "fk_standard_tool_created_user_id", "auth_user",
    on_delete = "PROTECT",
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
drop_foreign_key("stock_lists", "fk_stock_lists_category_id", ignore_errors=True)
```

| Parameter | Position | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `table` | 1st | string | Yes | Table containing the FK |
| `constraint_name` | 2nd | string | Yes | FK constraint name to drop |
| `ignore_errors` | keyword | bool | No | Suppress errors (default: `False`) |

### run_sql

Executes arbitrary SQL. Use for operations that don't have a dedicated builtin (extensions, data backfills, complex DDL).

```starlark
# Single-line SQL
run_sql(
    forward = "UPDATE core_data_file SET title = COALESCE(NULLIF(TRIM(original_file_name), ''), NULLIF(TRIM(core_description), ''))",
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
upsert_data("part_number_prefix",
    conflict_keys = ["id"],
    rows = [
        row(id="6476879f-0d2b-4cf4-a635-c0eb1a6fe98f", letter="A", description_1="'A' SERIES RADS"),
        row(id="13153dc9-e4b1-444c-892b-5dfb40eb1f6e", letter="B", description_1="'B' SERIES RADS"),
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
        create_table("users",
            fields = [
                uuid("id", nullable=True, primary_key=True, default="new_uuid"),
                varchar("email", 255),
                timestamp("created_date", default="now"),
            ],
        ),
    ],
)
```

### Table with Foreign Keys and Deferred Constraints

When a table has foreign keys, define the column with `foreign_key()` in `create_table`, then add the named constraint separately:

```starlark
migration(
    name = "0025_create_standard_tool",
    dependencies = ["0024_add_image_to_tmc_tml"],
    operations = [
        create_table("standard_tool",
            fields = [
                uuid("id", nullable=True, primary_key=True, default="new_uuid"),
                varchar("name", 60),
                varchar("source", 3, nullable=True),
                integer("display_order", default="zero"),
                timestamp("active_start_date", nullable=True),
                timestamp("active_end_date", nullable=True),
                timestamp("created_date"),
                timestamp("modified_date", nullable=True),
                foreign_key("created_user_id", fk("auth_user", on_delete="PROTECT"), nullable=True),
                foreign_key("modified_user_id", fk("auth_user", on_delete="PROTECT"), nullable=True),
            ],
        ),
        add_foreign_key("standard_tool", "created_user_id", "fk_standard_tool_created_user_id", "auth_user",
            on_delete = "PROTECT",
            ignore_errors = True,
        ),
        add_foreign_key("standard_tool", "modified_user_id", "fk_standard_tool_modified_user_id", "auth_user",
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
    name = "0007_add_title",
    dependencies = ["0006_add_app_version"],
    operations = [
        add_field("core_data_file", varchar("title", 255, nullable=True)),
        run_sql(
            forward = "UPDATE core_data_file SET title = COALESCE(NULLIF(TRIM(original_file_name), ''), NULLIF(TRIM(core_description), ''))",
        ),
        add_field("input_file", varchar("title", 255, nullable=True)),
        run_sql(
            forward = "UPDATE input_file SET title = COALESCE(NULLIF(TRIM(original_file_name), ''), NULLIF(TRIM(name), ''))",
        ),
    ],
)
```

### Widening a Column

Use `alter_field` with both old and new definitions:

```starlark
migration(
    name = "0015_widen_stock_lists_part_no_to_40",
    dependencies = ["0014_modify_fields"],
    operations = [
        alter_field("stock_lists",
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
    name = "0005_set_draft_status",
    dependencies = ["0004_core_calc_data"],
    operations = [
        upsert_data("code_status",
            conflict_keys = ["id"],
            rows = [
                row(
                    id="a1b2c3d4-0000-0000-0000-000000000001",
                    code="DRAFT",
                    description="Draft status",
                    active_start_date="2024-01-01 00:00:00.000000",
                    created_date="2024-01-01 00:00:00.000000",
                    created_user_id="55555555-5555-5555-5555-555555555555",
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
    name = "0025_cleanup",
    dependencies = ["0024_previous"],
    operations = [
        drop_foreign_key("stock_lists", "fk_stock_lists_category_id", ignore_errors=True),
        drop_field("stock_lists", "category_code"),
        drop_field("stock_lists", "category_id"),
    ],
)
```

### Blank Migration Template

`morphic generate --empty` creates a blank template:

```starlark
migration(
    name = "0015_custom",
    dependencies = ["0014_previous"],
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
    0002_add_audit_log.star
    0003_add_indexes.star
```

Files are loaded in alphabetical order, but execution order is determined by the dependency DAG, not filename sort order.
