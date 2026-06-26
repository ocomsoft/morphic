# Starlark Schema DSL Reference

Morphic schemas are defined in `schema.star` — a [Starlark](https://github.com/google/starlark-go) (Python-like) file that describes your database structure. When you run `morphic makemigrations`, morphic reads this file, diffs it against your migration history, and generates the appropriate migration files.

> **New project?** Use `schema.star`. The legacy `schema.yaml` format is still supported but Starlark is the default for all new projects. Run `morphic yaml2dsl` to convert an existing YAML schema.

---

## File Location

```
your-project/
├── schema/
│   └── schema.star          # Your schema definition
├── migrations/
│   ├── morphic.config.yaml  # Migration configuration
│   └── 0001_initial.star    # Generated migration files
```

---

## Complete Example

The following example shows the full range of DSL features. Refer back to this while reading the reference sections below.

```starlark
database("ecommerce", "2.0.0")

defaults("postgresql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "gen_random_uuid()",
    "zero":     "0",
    "true":     "true",
    "false":    "false",
})

defaults("mysql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "(UUID())",
    "zero":     "0",
    "true":     "1",
    "false":    "0",
})

type_mappings("postgresql", {
    "float": "DOUBLE PRECISION",
})

include("github.com/company/auth-module", "schemas/auth.star")

table("products",
    fields = [
        uuid("id",            primary_key=True, default="new_uuid"),
        varchar("name",       255,              nullable=False),
        text("description",   nullable=True),
        decimal("price",      10, 2,            nullable=False),
        integer("stock",      default="zero"),
        boolean("is_active",  default="true"),
        jsonb("metadata",     nullable=True),
        timestamp("created_at", default="now", auto_create=True),
        timestamp("updated_at", nullable=True,  auto_update=True),
    ],
    indexes = [
        index("idx_products_name", ["name"]),
    ],
)

table("categories",
    fields = [
        serial("id",          primary_key=True),
        varchar("name",       100,              nullable=False),
        varchar("slug",       100,              nullable=False),
        foreign_key("parent_id", fk("categories", on_delete="SET_NULL"), nullable=True),
    ],
    indexes = [
        index("idx_categories_slug", ["slug"], unique=True),
    ],
)

table("product_categories",
    fields = [
        serial("id",          primary_key=True),
        foreign_key("product_id",  fk("products",   on_delete="CASCADE"), nullable=False),
        foreign_key("category_id", fk("categories", on_delete="CASCADE"), nullable=False),
    ],
    indexes = [
        index("idx_product_categories_unique", ["product_id", "category_id"], unique=True),
    ],
)
```

---

## Root-Level Functions

A `schema.star` file calls these functions at the top level. Order matters: `database()` should come first, then `defaults()` and `type_mappings()`, then `include()`, then `table()` definitions.

### `database(name, version?)`

Declares the schema identity. This is required and must appear once.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Application or database name (used for tracking) |
| `version` | string | No | Schema version string (informational only) |

```starlark
database("myapp", "1.0.0")
```

The name and version appear in verbose output and have no effect on generated SQL.

---

### `defaults(db_type, mapping)`

Defines named default values for a specific database backend. Fields can reference these names by the symbolic key rather than writing database-specific SQL directly.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `db_type` | string | Yes | Database backend identifier (see supported values below) |
| `mapping` | dict | Yes | Map of symbolic name to SQL expression |

```starlark
defaults("postgresql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "gen_random_uuid()",
    "today":    "CURRENT_DATE",
    "zero":     "0",
    "true":     "true",
    "false":    "false",
})

defaults("mysql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "(UUID())",
    "today":    "(CURDATE())",
    "zero":     "0",
    "true":     "1",
    "false":    "0",
})

defaults("sqlserver", {
    "blank":    "",
    "now":      "GETDATE()",
    "new_uuid": "NEWID()",
    "today":    "CAST(GETDATE() AS DATE)",
    "zero":     "0",
    "true":     "1",
    "false":    "0",
})
```

**Why use symbolic defaults?** Writing `default="now"` keeps your schema portable. Morphic substitutes the correct SQL expression for each backend at migration-generation time. Without this, you would need to write `CURRENT_TIMESTAMP`, `GETDATE()`, or `CURRENT_TIMESTAMP` depending on the target database.

**Built-in defaults** are provided automatically for each database and do not need to be declared unless you want to override them:

| Symbolic Key | PostgreSQL | MySQL | SQLite | SQL Server |
|--------------|------------|-------|--------|------------|
| `blank` | `''` | `''` | `''` | `''` |
| `now` | `CURRENT_TIMESTAMP` | `CURRENT_TIMESTAMP` | `CURRENT_TIMESTAMP` | `GETDATE()` |
| `today` | `CURRENT_DATE` | `(CURDATE())` | `CURRENT_DATE` | `CAST(GETDATE() AS DATE)` |
| `new_uuid` | `gen_random_uuid()` | `(UUID())` | `''` | `NEWID()` |
| `zero` | `"0"` | `"0"` | `"0"` | `"0"` |
| `true` | `"true"` | `"1"` | `"1"` | `"1"` |
| `false` | `"false"` | `"0"` | `"0"` | `"0"` |

You can add your own entries:

```starlark
defaults("postgresql", {
    "active_status": "'active'",
    "default_role":  "'viewer'",
})
```

---

### `type_mappings(db_type, mapping)`

Overrides the SQL type that morphic generates for an abstract field type on a specific database. Use this when the built-in type mapping is not suitable.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `db_type` | string | Yes | Database backend identifier |
| `mapping` | dict | Yes | Map of abstract type name to SQL type string |

```starlark
type_mappings("postgresql", {
    "float": "DOUBLE PRECISION",  # override REAL → DOUBLE PRECISION
    "text":  "CITEXT",            # use case-insensitive text extension
})

type_mappings("sqlserver", {
    "text": "NVARCHAR(MAX)",       # ensure unicode text
})

type_mappings("mysql", {
    "uuid": "CHAR(36)",            # explicit char length for UUIDs
})
```

**Parameterised types** use Go template syntax to access field attributes:

```starlark
type_mappings("postgresql", {
    "decimal": "NUMERIC({{.Precision}},{{.Scale}})",
    "varchar": "CHARACTER VARYING({{.Length}})",
})
```

Available template variables: `.Length`, `.Precision`, `.Scale`.

**Built-in type mappings** (what morphic uses when no override is provided):

| Schema Type | PostgreSQL | MySQL | SQLite | SQL Server |
|-------------|------------|-------|--------|------------|
| `varchar` | `VARCHAR(n)` | `VARCHAR(n)` | `TEXT` | `VARCHAR(n)` |
| `text` | `TEXT` | `TEXT` | `TEXT` | `NVARCHAR(MAX)` |
| `integer` | `INTEGER` | `INT` | `INTEGER` | `INT` |
| `bigint` | `BIGINT` | `BIGINT` | `INTEGER` | `BIGINT` |
| `serial` | `SERIAL` | `INT AUTO_INCREMENT` | `INTEGER` | `INT IDENTITY` |
| `float` | `REAL` | `FLOAT` | `REAL` | `FLOAT` |
| `decimal` | `DECIMAL(p,s)` | `DECIMAL(p,s)` | `NUMERIC` | `DECIMAL(p,s)` |
| `boolean` | `BOOLEAN` | `TINYINT(1)` | `INTEGER` | `BIT` |
| `timestamp` | `TIMESTAMP` | `TIMESTAMP` | `DATETIME` | `DATETIME2` |
| `date` | `DATE` | `DATE` | `DATE` | `DATE` |
| `time` | `TIME` | `TIME` | `TIME` | `TIME` |
| `uuid` | `UUID` | `CHAR(36)` | `TEXT` | `UNIQUEIDENTIFIER` |
| `jsonb` | `JSONB` | `JSON` | `TEXT` | `NVARCHAR(MAX)` |
| `bytes` | `BYTEA` | `BLOB` | `BLOB` | `VARBINARY(MAX)` |

---

### `include(module, path)`

Imports table definitions from an external Go module. This allows schema modularization and reuse across projects.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `module` | string | Yes | Go module path (e.g. `github.com/company/auth-module`) |
| `path` | string | Yes | Path to the `schema.star` within that module |

```starlark
include("github.com/company/auth-module",  "schemas/auth.star")
include("github.com/company/audit-module", "schemas/audit.star")
```

**How module resolution works:**

1. Morphic first checks Go workspace modules (`go.work`)
2. Falls back to `go.mod` dependencies and the module cache
3. Included schemas can have their own includes, forming a dependency tree
4. Circular dependencies are detected and skipped automatically
5. When the main schema and an included schema define the same table, the main schema wins

---

### `table(name, fields=[], indexes=[])`

Declares a database table. Each call appends one table to the schema.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Table name (snake_case recommended) |
| `fields` | list of field dicts | No | Columns in the table, in order |
| `indexes` | list of index dicts | No | Indexes on the table |

```starlark
table("users",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        varchar("email", 255, nullable=False),
        varchar("username", 100, nullable=False),
        boolean("is_active", default="true"),
        timestamp("created_at", default="now", auto_create=True),
        timestamp("updated_at", nullable=True, auto_update=True),
    ],
    indexes = [
        index("idx_users_email",    ["email"],    unique=True),
        index("idx_users_username", ["username"], unique=True),
    ],
)
```

---

## Field Functions

Fields are declared inside the `fields` list of a `table()` call. Morphic provides **typed field builtins** (preferred) for every common type, and a generic `field()` fallback for custom or unusual types.

### Common Optional Parameters

All field functions accept these optional keyword arguments unless noted otherwise:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `nullable` | bool | `True` | Whether the column accepts `NULL` |
| `primary_key` | bool | `False` | Whether this column is the primary key |
| `default` | string | `""` | Default value — a symbolic key from `defaults()` or a literal SQL expression |

---

### `uuid(name, **kwargs)`

A UUID/GUID column. Maps to `UUID` on PostgreSQL, `CHAR(36)` on MySQL, `UNIQUEIDENTIFIER` on SQL Server.

```starlark
uuid("id", primary_key=True, default="new_uuid")
uuid("correlation_id", nullable=True)
```

---

### `varchar(name, length, **kwargs)`

A variable-length string column with a required maximum length.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Column name |
| `length` | int | Yes | Maximum character length |

```starlark
varchar("email",    255, nullable=False)
varchar("username", 100, nullable=False)
varchar("bio",      500, nullable=True, default="blank")
```

---

### `text(name, **kwargs)`

An unbounded text column. Use for large content where a character limit is unnecessary.

```starlark
text("description", nullable=True)
text("notes",       nullable=True, default="blank")
```

---

### `integer(name, **kwargs)`

A 32-bit signed integer column.

```starlark
integer("age",      nullable=True)
integer("quantity", nullable=False, default="zero")
```

---

### `bigint(name, **kwargs)`

A 64-bit signed integer column. Use for large counters or IDs that may exceed 2 billion.

```starlark
bigint("view_count", default="zero")
bigint("file_size",  nullable=True)
```

---

### `serial(name, **kwargs)`

An auto-incrementing integer column. Typically used as a primary key. Maps to `SERIAL` on PostgreSQL, `INT AUTO_INCREMENT` on MySQL, `INT IDENTITY` on SQL Server.

```starlark
serial("id", primary_key=True)
```

---

### `float(name, **kwargs)`

A floating-point number column. Maps to `REAL` on PostgreSQL and SQLite. Do not use for monetary values — use `decimal` instead.

```starlark
float("latitude")
float("score", nullable=True)
```

---

### `decimal(name, precision, scale, **kwargs)`

A fixed-point decimal column with exact numeric precision. Required for monetary values.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Column name |
| `precision` | int | Yes | Total number of significant digits |
| `scale` | int | Yes | Number of digits after the decimal point |

```starlark
decimal("price",         10, 2, nullable=False)
decimal("tax_rate",       5, 4, nullable=True)
decimal("total_amount",  15, 2, default="zero")
```

---

### `boolean(name, **kwargs)`

A boolean true/false column. Maps to `BOOLEAN` on PostgreSQL, `TINYINT(1)` on MySQL, `BIT` on SQL Server.

```starlark
boolean("is_active",   default="true")
boolean("is_verified", default="false")
boolean("is_deleted",  nullable=True)
```

---

### `timestamp(name, **kwargs)`

A date-and-time column. Accepts two additional keyword arguments for automatic timestamping:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `auto_create` | bool | `False` | Set the column to the current time on INSERT |
| `auto_update` | bool | `False` | Set the column to the current time on UPDATE |

```starlark
timestamp("created_at", default="now",  auto_create=True)
timestamp("updated_at", nullable=True,  auto_update=True)
timestamp("deleted_at", nullable=True)
timestamp("scheduled_at", nullable=False, default="now")
```

---

### `date(name, **kwargs)`

A date-only column (no time component).

```starlark
date("birth_date",   nullable=True)
date("start_date",   nullable=False, default="today")
date("expiry_date",  nullable=True)
```

---

### `time(name, **kwargs)`

A time-of-day column (no date component).

```starlark
time("opens_at",  nullable=False)
time("closes_at", nullable=False)
```

---

### `jsonb(name, **kwargs)`

A JSON data column. Maps to `JSONB` on PostgreSQL (binary JSON with indexing support), `JSON` on MySQL, and `TEXT` on SQLite and SQL Server.

```starlark
jsonb("metadata",    nullable=True)
jsonb("settings",    nullable=True, default="blank")
jsonb("permissions", nullable=False)
```

---

### `bytes(name, **kwargs)`

A binary data column. Maps to `BYTEA` on PostgreSQL, `BLOB` on MySQL and SQLite, `VARBINARY(MAX)` on SQL Server.

```starlark
bytes("avatar",    nullable=True)
bytes("thumbnail", nullable=True)
```

---

### `foreign_key(name, fk_dict, **kwargs)`

A foreign key column referencing another table. The column itself stores the referenced row's primary key value. Morphic generates an integer (or UUID-typed, depending on the referenced table's PK) column plus the constraint.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Column name |
| `fk` | fk dict | Yes | A `fk()` call describing the relationship |

```starlark
foreign_key("user_id",     fk("users",    on_delete="CASCADE"),   nullable=False)
foreign_key("category_id", fk("categories", on_delete="RESTRICT"), nullable=False)
foreign_key("parent_id",   fk("categories", on_delete="SET_NULL"), nullable=True)
```

---

### `field(name, type, **kwargs)`

The generic field builder used when no typed builtin exists for the desired type. It accepts all parameters that typed builtins accept, plus `foreign_key` and `many_to_many` as keyword arguments.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Column name |
| `type` | string | Yes | Any valid field type string |
| `length` | int | No | Maximum length (varchar, text) |
| `precision` | int | No | Total digits (decimal) |
| `scale` | int | No | Decimal places (decimal) |
| `auto_create` | bool | No | Auto-set on INSERT (timestamp, date, time) |
| `auto_update` | bool | No | Auto-set on UPDATE (timestamp, date, time) |
| `foreign_key` | fk dict | No | A `fk()` call for FK columns |
| `many_to_many` | string | No | Target table name for M2M columns |

```starlark
# Equivalent to: varchar("status", 50, nullable=False, default="pending")
field("status", "varchar", length=50, nullable=False, default="pending")

# Custom type that has no typed builtin
field("geom", "geometry", nullable=True)
```

---

## Relationship Functions

### `fk(table, on_delete?, on_update?)`

Constructs a foreign key descriptor. Always used as the `fk` argument to `foreign_key()` or the `foreign_key` kwarg of `field()`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `table` | string | Yes | The table being referenced |
| `on_delete` | string | No | Action when the referenced row is deleted |
| `on_update` | string | No | Action when the referenced row's PK is updated |

```starlark
fk("users",      on_delete="CASCADE")
fk("categories", on_delete="RESTRICT")
fk("departments", on_delete="SET_NULL")
fk("users",      on_delete="CASCADE", on_update="CASCADE")
```

**`on_delete` / `on_update` options:**

| Value | Behaviour |
|-------|-----------|
| `CASCADE` | Automatically delete or update referencing rows |
| `RESTRICT` | Prevent deletion/update while references exist (checked immediately) |
| `SET_NULL` | Set the FK column to `NULL` (column must be nullable) |
| `PROTECT` | Prevent deletion/update while references exist (default) |
| `NO ACTION` | Database default — deferred referential integrity check |

---

### Many-to-Many Relationships

Morphic does not have a dedicated many-to-many table type. Model many-to-many relationships with an explicit junction table containing two `foreign_key` columns. This gives full control over the junction table and allows adding payload columns when the relationship carries data.

```starlark
table("tags",
    fields = [
        serial("id",  primary_key=True),
        varchar("name", 100, nullable=False),
    ],
)

table("post_tags",
    fields = [
        serial("id",     primary_key=True),
        foreign_key("post_id", fk("posts", on_delete="CASCADE"), nullable=False),
        foreign_key("tag_id",  fk("tags",  on_delete="CASCADE"), nullable=False),
    ],
    indexes = [
        index("idx_post_tags_unique", ["post_id", "tag_id"], unique=True),
    ],
)
```

If you need to express a `many_to_many` metadata annotation on a field (for tooling or code generators), use the `field()` generic builder with the `many_to_many` kwarg:

```starlark
field("tags", "many_to_many", many_to_many="tags")
```

---

## Index Function

### `index(name, fields, unique?, method?, where?)`

Defines a database index on one or more columns. Use inside the `indexes` list of a `table()` call.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | Yes | Unique index name within the database |
| `fields` | list of strings | Yes | Column names to include in the index |
| `unique` | bool | No | Create a unique index (default `False`) |
| `method` | string | No | Index access method (PostgreSQL only — e.g. `BTREE`, `HASH`, `GIN`, `GIST`, `BRIN`) |
| `where` | string | No | Partial index predicate (not supported by MySQL) |

```starlark
# Simple lookup index
index("idx_users_email", ["email"])

# Unique constraint
index("idx_users_email_unique", ["email"], unique=True)

# Composite index for a common query filter
index("idx_orders_user_status", ["user_id", "status"])

# PostgreSQL GIN index for full-text search
index("idx_products_search", ["search_vector"], method="GIN")

# Partial index — only index active records
index("idx_users_active_email", ["email"],
    unique=True,
    where="is_active = TRUE",
)
```

**Index naming convention:** prefix with `idx_`, include the table name and the column names: `idx_tablename_column1_column2`.

**Database-specific notes:**

- `method` is silently ignored on SQLite and SQL Server (no partial index support)
- `where` is silently ignored on MySQL
- Indexes for foreign key columns are automatically managed by morphic — you do not need to declare them manually

---

## Supported Databases

The `db_type` string used in `defaults()` and `type_mappings()` must be one of:

| Value | Database |
|-------|----------|
| `postgresql` | PostgreSQL |
| `mysql` | MySQL |
| `sqlserver` | Microsoft SQL Server |
| `sqlite` | SQLite |
| `redshift` | Amazon Redshift |
| `clickhouse` | ClickHouse |
| `tidb` | TiDB |
| `vertica` | Vertica |
| `ydb` | YDB |
| `turso` | Turso |
| `starrocks` | StarRocks |
| `auroradsql` | Aurora DSQL |

---

## Field Types Reference

All valid values for the `type` parameter of the generic `field()` function:

| Type | Description | Required Params |
|------|-------------|-----------------|
| `uuid` | UUID/GUID | — |
| `varchar` | Variable-length string | `length` |
| `text` | Unbounded text | — |
| `integer` | 32-bit signed integer | — |
| `bigint` | 64-bit signed integer | — |
| `serial` | Auto-incrementing integer | — |
| `float` | Floating-point number | — |
| `decimal` | Fixed-point decimal | `precision`, `scale` |
| `boolean` | Boolean true/false | — |
| `timestamp` | Date and time | — |
| `date` | Date only | — |
| `time` | Time of day only | — |
| `jsonb` | JSON document | — |
| `bytes` | Binary data | — |
| `foreign_key` | FK reference column | `foreign_key` |
| `many_to_many` | M2M annotation | `many_to_many` |
| `text[]` | Text array (PostgreSQL) | — |

---

## Realistic Multi-Table Example

The following schema models a simple content management system with users, posts, tags, and comments. It demonstrates all major features: UUID primary keys, foreign keys, self-referencing, many-to-many via junction table, partial indexes, and cross-database defaults.

```starlark
database("cms", "1.0.0")

defaults("postgresql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "gen_random_uuid()",
    "true":     "true",
    "false":    "false",
})

defaults("mysql", {
    "blank":    "",
    "now":      "CURRENT_TIMESTAMP",
    "new_uuid": "(UUID())",
    "true":     "1",
    "false":    "0",
})

# ── Users ────────────────────────────────────────────────────────────────────

table("users",
    fields = [
        uuid("id",            primary_key=True, default="new_uuid"),
        varchar("email",      255,  nullable=False),
        varchar("username",   100,  nullable=False),
        varchar("password_hash", 255, nullable=False),
        boolean("is_active",  default="true"),
        boolean("is_staff",   default="false"),
        timestamp("created_at", default="now", auto_create=True),
        timestamp("updated_at", nullable=True,  auto_update=True),
    ],
    indexes = [
        index("idx_users_email",    ["email"],    unique=True),
        index("idx_users_username", ["username"], unique=True),
    ],
)

# ── Tags ─────────────────────────────────────────────────────────────────────

table("tags",
    fields = [
        serial("id",        primary_key=True),
        varchar("name",     100, nullable=False),
        varchar("slug",     100, nullable=False),
    ],
    indexes = [
        index("idx_tags_slug", ["slug"], unique=True),
    ],
)

# ── Posts ─────────────────────────────────────────────────────────────────────

table("posts",
    fields = [
        uuid("id",          primary_key=True, default="new_uuid"),
        foreign_key("author_id", fk("users", on_delete="CASCADE"), nullable=False),
        varchar("title",    255,  nullable=False),
        varchar("slug",     255,  nullable=False),
        text("body",        nullable=False),
        varchar("status",   50,   nullable=False, default="blank"),
        boolean("is_published", default="false"),
        timestamp("published_at", nullable=True),
        timestamp("created_at", default="now", auto_create=True),
        timestamp("updated_at", nullable=True,  auto_update=True),
    ],
    indexes = [
        index("idx_posts_slug",   ["slug"],   unique=True),
        index("idx_posts_author", ["author_id"]),
        index("idx_posts_published", ["published_at"],
            where="is_published = TRUE",
        ),
    ],
)

# ── Post Tags (junction table for posts ↔ tags many-to-many) ─────────────────

table("post_tags",
    fields = [
        serial("id",      primary_key=True),
        foreign_key("post_id", fk("posts", on_delete="CASCADE"), nullable=False),
        foreign_key("tag_id",  fk("tags",  on_delete="CASCADE"), nullable=False),
    ],
    indexes = [
        index("idx_post_tags_unique", ["post_id", "tag_id"], unique=True),
    ],
)

# ── Comments (self-referencing for threaded replies) ─────────────────────────

table("comments",
    fields = [
        uuid("id",          primary_key=True, default="new_uuid"),
        foreign_key("post_id",   fk("posts",    on_delete="CASCADE"),  nullable=False),
        foreign_key("author_id", fk("users",    on_delete="CASCADE"),  nullable=False),
        foreign_key("parent_id", fk("comments", on_delete="SET_NULL"), nullable=True),
        text("body",        nullable=False),
        boolean("is_approved", default="false"),
        timestamp("created_at", default="now", auto_create=True),
    ],
    indexes = [
        index("idx_comments_post",   ["post_id"]),
        index("idx_comments_author", ["author_id"]),
        index("idx_comments_parent", ["parent_id"]),
    ],
)
```

---

## Converting from YAML

If you have an existing `schema.yaml`, convert it to Starlark in one step:

```bash
morphic yaml2dsl --schema ./schema
```

This generates a `schema.star` alongside your existing `schema.yaml`. Review the output, then remove `schema.yaml` — morphic will error if both files exist in the same directory.

---

## Validation Rules

Morphic validates your schema before generating migrations. Errors are reported with table and field names.

- `database()` name must not be empty
- Every `table()` must have a `name` and at least one field
- Every field must have a `name` and a valid `type`
- `varchar` fields must have a positive `length`
- `decimal` fields must have a positive `precision` and a `scale` between `0` and `precision`
- `foreign_key` fields must supply an `fk()` dict with a non-empty `table`
- `many_to_many` fields must supply a non-empty target table string
- Index fields must all exist on the table where the index is defined
- Each schema directory must contain exactly one schema file (`schema.star` or `schema.yaml`, not both)

---

## Best Practices

**Naming conventions**

- Tables: plural, `snake_case` — `users`, `product_categories`
- Fields: `snake_case` — `created_at`, `is_active`
- Foreign key columns: `{referenced_table_singular}_id` — `user_id`, `category_id`
- Indexes: `idx_{table}_{column1}_{column2}` — `idx_users_email_username`

**Primary keys**

- Prefer `uuid` primary keys for distributed systems or services that need to generate IDs without a database round-trip
- Prefer `serial` primary keys for simple, single-database applications where auto-increment is sufficient

**Timestamps**

Every table that represents a business entity should have audit timestamps:

```starlark
timestamp("created_at", default="now", auto_create=True),
timestamp("updated_at", nullable=True,  auto_update=True),
```

**Nullable columns**

New columns added to an existing table with rows must be nullable (or have a default) or the migration will fail at runtime. Add new required columns as nullable first, backfill the data, then alter to not-null in a follow-up migration.

**Monetary values**

Always use `decimal`, never `float`, for currency or any value requiring exact arithmetic:

```starlark
decimal("price", 10, 2, nullable=False)   # up to 99,999,999.99
decimal("amount", 15, 2, nullable=False)  # up to 9,999,999,999,999.99
```

---

## See Also

- [Starlark Migration Format](starlark-migrations.md) — migration `.star` files (separate from schema `.star` files)
- [Schema Format (YAML)](schema-format.md) — legacy YAML schema reference
- [Configuration Guide](configuration.md) — `morphic.config.yaml` options
- [Commands](commands/) — CLI command reference
