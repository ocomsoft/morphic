# morphic Usage Guide

A practical, end-to-end walkthrough of the morphic workflow using PostgreSQL. This guide covers everything from initial project setup through schema evolution, SQL inspection, data seeding, and stored procedures — including custom type mappings and custom defaults.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Project Setup](#project-setup)
3. [Creating the Initial Schema](#creating-the-initial-schema)
4. [Generating the Initial Migration](#generating-the-initial-migration)
5. [Checking the Generated SQL](#checking-the-generated-sql)
6. [Applying Migrations](#applying-migrations)
7. [Adding a Table](#adding-a-table)
8. [Adding Fields to an Existing Table](#adding-fields-to-an-existing-table)
9. [Adding Indexes](#adding-indexes)
10. [Altering Fields](#altering-fields)
11. [Removing a Field](#removing-a-field)
12. [Removing a Table](#removing-a-table)
13. [Inserting Seed Data](#inserting-seed-data)
14. [Adding a Stored Procedure](#adding-a-stored-procedure)
15. [Rolling Back](#rolling-back)
16. [Day-to-Day Workflow Summary](#day-to-day-workflow-summary)

---

## Prerequisites

- Go 1.24 or later
- morphic installed: `go install github.com/ocomsoft/morphic@latest`
- A running PostgreSQL instance
- `DATABASE_URL` environment variable set, for example:

```bash
export DATABASE_URL="postgresql://dev_user:dev_pass@localhost:5432/myapp_dev"
```

In the Postgres Server run the following
```sql
CREATE ROLE dev_user LOGIN
  PASSWORD 'dev_pass'
  NOSUPERUSER INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION;

CREATE DATABASE myapp_dev
  WITH OWNER = dev_user
       ENCODING = 'UTF8'
       TABLESPACE = pg_default
       LC_COLLATE = 'en_US.utf8'
       LC_CTYPE = 'en_US.utf8'
       CONNECTION LIMIT = -1;

USE myapp_dev;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
```
---

## Project Setup

Initialise the project from your Go application root. This creates the `migrations/` directory and a configuration file.

```bash
morphic init --database postgresql
```

This produces:

```
myapp/
├── go.mod
├── schema/                     # you create this directory and schema.star
└── migrations/
    └── morphic.config.yaml
```

Create the schema directory:

```bash
mkdir -p schema
```

---

## Creating the Initial Schema

The schema is defined in `schema/schema.star`. This file describes every table, field, index, and relationship in a database-agnostic way using the Starlark DSL.

The example below demonstrates:

- **Custom defaults** — named aliases for database-specific SQL expressions
- **Custom type mappings** — override how schema field types translate to PostgreSQL SQL types
- **UUID primary keys** using `gen_random_uuid()`
- **Timestamps** that auto-populate on create and update
- **Decimal** fields for monetary values
- **JSONB** fields for flexible metadata
- **Foreign keys** with cascading deletes
- **Indexes** for query performance

Create `schema/schema.star`:

```starlark
database("myapp", "1.0.0")

# ---------------------------------------------------------------------------
# Custom type mappings — override the default SQL types for this schema.
# This is useful when you want to use a PostgreSQL-specific type instead of
# the generic default (e.g., CITEXT for case-insensitive text, MONEY for
# currency, DOUBLE PRECISION for higher-precision floats).
# ---------------------------------------------------------------------------
# Use CITEXT so email comparisons are case-insensitive without lower()
# Use DOUBLE PRECISION instead of the default REAL for floats
set_type_mappings({"text": "CITEXT", "float": "DOUBLE PRECISION"})

# ---------------------------------------------------------------------------
# Custom defaults — named aliases for database-specific SQL expressions.
# Fields reference these by name (e.g., default="new_uuid") rather than
# embedding raw SQL in the schema, keeping the schema database-agnostic.
# ---------------------------------------------------------------------------
defaults("postgresql", {
    "blank":          "''",
    "now":            "CURRENT_TIMESTAMP",
    "new_uuid":       "gen_random_uuid()",
    "today":          "CURRENT_DATE",
    "zero":           "0",
    "true":           "true",
    "false":          "false",
    "empty_json":     "'{}'",
    # Custom defaults specific to this project
    "default_status": "'active'",
    "default_role":   "'member'",
})

# -------------------------------------------------------------------------
# users — core account table
# -------------------------------------------------------------------------
table("users",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),   # resolves to gen_random_uuid() at runtime
        text("email"),                                        # maps to CITEXT (see set_type_mappings above)
        varchar("username", 100),
        varchar("password_hash", 255),
        varchar("role", 50, default="default_role"),          # resolves to 'member'
        varchar("status", 50, default="default_status"),      # resolves to 'active'
        jsonb("metadata", nullable=True, default="empty_json"),  # resolves to '{}'
        timestamp("created_at", default="now", auto_create=True),  # automatically set on INSERT
        timestamp("updated_at", nullable=True, auto_update=True),  # automatically set on UPDATE
    ],
    indexes = [
        index("idx_users_email",    ["email"],    unique=True),
        index("idx_users_username", ["username"], unique=True),
        index("idx_users_status",   ["status"]),
    ],
)

# -------------------------------------------------------------------------
# categories — product categories (self-referencing for hierarchy)
# -------------------------------------------------------------------------
table("categories",
    fields = [
        serial("id", primary_key=True),
        varchar("name", 100),
        varchar("slug", 100),
        foreign_key("parent_id", fk("categories", on_delete="SET_NULL"), nullable=True),  # removing a parent keeps the children
    ],
    indexes = [
        index("idx_categories_slug", ["slug"], unique=True),
    ],
)

# -------------------------------------------------------------------------
# products — items for sale
# -------------------------------------------------------------------------
table("products",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        varchar("name", 255),
        text("description", nullable=True),
        decimal("price", 10, 2),
        float("weight_kg", nullable=True),   # maps to DOUBLE PRECISION (see set_type_mappings)
        integer("stock_count", default="zero"),  # resolves to 0
        boolean("is_active", default="true"),    # resolves to true
        jsonb("metadata", nullable=True, default="empty_json"),
        timestamp("created_at", default="now", auto_create=True),
        timestamp("updated_at", nullable=True, auto_update=True),
    ],
    indexes = [
        index("idx_products_name",              ["name"]),
        index("idx_products_is_active_created", ["is_active", "created_at"]),
    ],
)

# -------------------------------------------------------------------------
# product_categories — many-to-many junction table
# -------------------------------------------------------------------------
table("product_categories",
    fields = [
        serial("id", primary_key=True),
        foreign_key("product_id",  fk("products",   on_delete="CASCADE")),
        foreign_key("category_id", fk("categories", on_delete="CASCADE")),
    ],
    indexes = [
        index("idx_product_categories_unique", ["product_id", "category_id"], unique=True),  # prevents duplicate associations
    ],
)
```

---

## Generating the Initial Migration

With the schema defined, generate the first migration file:

```bash
morphic generate --name "initial"
```

Output:

```
Created migrations/0001_initial.star
```

The generated file (`migrations/0001_initial.star`) looks like this:

```starlark
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        set_type_mappings({"text": "CITEXT", "float": "DOUBLE PRECISION"}),
        set_defaults({
            "blank":          "''",
            "now":            "CURRENT_TIMESTAMP",
            "new_uuid":       "gen_random_uuid()",
            "today":          "CURRENT_DATE",
            "zero":           "0",
            "true":           "true",
            "false":          "false",
            "empty_json":     "'{}'",
            "default_status": "'active'",
            "default_role":   "'member'",
        }),
        create_table("users",
            fields = [
                uuid("id",            primary_key=True, default="new_uuid"),
                text("email"),
                varchar("username",      100),
                varchar("password_hash", 255),
                varchar("role",          50,  default="default_role"),
                varchar("status",        50,  default="default_status"),
                jsonb("metadata",        nullable=True, default="empty_json"),
                timestamp("created_at", default="now", auto_create=True),
                timestamp("updated_at", nullable=True, auto_update=True),
            ],
            indexes = [
                index("idx_users_email",    ["email"],    unique=True),
                index("idx_users_username", ["username"], unique=True),
                index("idx_users_status",   ["status"]),
            ],
        ),
        # ... categories, products, product_categories create_table ops
    ],
)
```

> **Note:** `set_defaults` and `set_type_mappings` are prepended automatically whenever your schema defines those sections. They carry no SQL — they record the configuration in the migration DAG so subsequent migrations and `showsql` use the correct values.

---

## Checking the Generated SQL

Before applying anything, inspect the SQL that will be executed. There are two ways to do this.

### Option 1 — dump-sql (schema preview, no migration state)

`schema-to-sql` shows the CREATE TABLE statements that your Starlark schema would generate, without consulting the migration history at all:

```bash
morphic schema-to-sql --database postgresql
```

Output:

```sql
-- Database: myapp (v1.0.0)
-- Target: postgresql

CREATE TABLE users (
    id         UUID         NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    email      CITEXT       NOT NULL,
    username   VARCHAR(100) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role       VARCHAR(50)  NOT NULL DEFAULT 'member',
    status     VARCHAR(50)  NOT NULL DEFAULT 'active',
    metadata   JSONB        DEFAULT '{}',
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_users_email    ON users (email);
CREATE UNIQUE INDEX idx_users_username ON users (username);
CREATE        INDEX idx_users_status   ON users (status);

CREATE TABLE categories (
    id        SERIAL PRIMARY KEY,
    name      VARCHAR(100) NOT NULL,
    slug      VARCHAR(100) NOT NULL,
    parent_id INTEGER REFERENCES categories(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_categories_slug ON categories (slug);

CREATE TABLE products (
    id          UUID           NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255)   NOT NULL,
    description CITEXT,
    price       DECIMAL(10,2)  NOT NULL,
    weight_kg   DOUBLE PRECISION,
    stock_count INTEGER        NOT NULL DEFAULT 0,
    is_active   BOOLEAN        NOT NULL DEFAULT true,
    metadata    JSONB          DEFAULT '{}',
    created_at  TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP
);

CREATE INDEX idx_products_name              ON products (name);
CREATE INDEX idx_products_is_active_created ON products (is_active, created_at);

CREATE TABLE product_categories (
    id          SERIAL  PRIMARY KEY,
    product_id  UUID    NOT NULL REFERENCES products(id)   ON DELETE CASCADE,
    category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_product_categories_unique ON product_categories (product_id, category_id);
```

Notice that:
- `text` fields render as `CITEXT` (from `type_mappings`)
- `float` fields render as `DOUBLE PRECISION` (from `type_mappings`)
- `default: new_uuid` resolves to `gen_random_uuid()` (from `defaults`)
- `default: default_role` resolves to `'member'`

### Option 2 — showsql (pending-migration SQL)

After generating the migration file, `showsql` shows exactly what SQL `morphic migrate up` will execute. This includes only the pending migrations:

```bash
morphic migrate showsql
```

Output:

```sql
-- 0001_initial
CREATE TABLE users ( ... );
CREATE UNIQUE INDEX idx_users_email ON users (email);
-- ... all DDL for all tables and indexes
```

Use `showsql` for final review before production deployments.

---

## Applying Migrations

`morphic migrate` loads the migration `.star` files in-process via the Starlark interpreter and runs them — no `go build`, no temporary binary:

```bash
morphic migrate up
```

Output:

```
Applying 0001_initial... done
```

Check the applied state:

```bash
morphic migrate status
```

Output:

```
Migration            Status
---------------------------------
0001_initial         Applied
```

---

## Adding a Table

Suppose you want to add an `orders` table. Edit `schema/schema.star` and add the new table definition:

```starlark
# -------------------------------------------------------------------------
# orders — customer orders
# -------------------------------------------------------------------------
table("orders",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        foreign_key("user_id", fk("users", on_delete="RESTRICT")),  # prevent deleting users with orders
        varchar("status", 50, default="default_status"),             # resolves to 'active'
        decimal("total_amount", 12, 2, default="zero"),
        text("notes", nullable=True),                                # CITEXT via set_type_mappings
        timestamp("placed_at", default="now", auto_create=True),
    ],
    indexes = [
        index("idx_orders_user_id",      ["user_id"]),
        index("idx_orders_status_placed", ["status", "placed_at"]),
    ],
)
```

Generate the migration:

```bash
morphic generate --name "add_orders"
```

Output:

```
Created migrations/0002_add_orders.star
```

The generated file contains a single `create_table` operation. Review then apply:

```bash
morphic migrate showsql
morphic migrate up
```

---

## Adding Fields to an Existing Table

Add a `phone` field and a `last_login_at` timestamp to the `users` table by editing `schema/schema.star`:

```starlark
table("users",
    fields = [
        # ... existing fields ...
        varchar("phone", 30, nullable=True),           # nullable so existing rows are not affected
        timestamp("last_login_at", nullable=True),
    ],
)
```

Generate and apply:

```bash
morphic generate --name "add_user_phone_and_last_login"
morphic migrate up
```

The generated migration uses `add_field` for each new column:

```starlark
migration(
    name = "0003_add_user_phone_and_last_login",
    dependencies = ["0002_add_orders"],
    operations = [
        add_field("users", varchar("phone", 30, nullable=True)),
        add_field("users", timestamp("last_login_at", nullable=True)),
    ],
)
```

> **Tip:** Always add new columns as `nullable: true` or with a default value when the table already has rows. Otherwise the `ALTER TABLE ADD COLUMN` will fail on databases that enforce NOT NULL immediately.

---

## Adding Indexes

Add a composite index on `users(role, status)` for a query that filters by both. Edit `schema/schema.star` to add the index to the `users` table:

```starlark
table("users",
    fields = [ # ... existing fields ... ],
    indexes = [
        # ... existing indexes ...
        index("idx_users_role_status", ["role", "status"]),
    ],
)
```

Generate and apply:

```bash
morphic generate --name "add_user_role_status_index"
morphic migrate up
```

Generated operation:

```starlark
migration(
    name = "0004_add_user_role_status_index",
    dependencies = ["0003_add_user_phone_and_last_login"],
    operations = [
        add_index("users",
            index("idx_users_role_status", ["role", "status"]),
        ),
    ],
)
```

---

## Altering Fields

Suppose you need to expand `status` from `varchar(50)` to `varchar(100)` on the `users` table, and make `phone` non-nullable (after a data backfill).

### Simple type change — expand varchar length

Update the field in `schema/schema.star`:

```starlark
varchar("status", 100, default="default_status"),   # was 50
```

Generate:

```bash
morphic generate --name "expand_user_status_length"
```

Generated operation:

```starlark
alter_field("users",
    old_field = varchar("status", 50,  default="default_status"),
    new_field = varchar("status", 100, default="default_status"),
)
```

### Safe NOT NULL migration — add column, backfill, tighten

For `phone`, edit the schema to remove `nullable=True`:

```starlark
varchar("phone", 30, default="blank"),   # resolves to ''
```

After generating, edit the migration file **before applying** to insert a backfill step:

```starlark
# migrations/0005_make_phone_required.star
migration(
    name = "0005_make_phone_required",
    dependencies = ["0004_add_user_role_status_index"],
    operations = [
        # Step 1: backfill any NULL values with an empty string
        run_sql(
            forward = "UPDATE users SET phone = '' WHERE phone IS NULL",
            # intentionally no backward — irreversible
        ),
        # Step 2: tighten to NOT NULL
        alter_field("users",
            old_field = varchar("phone", 30, nullable=True),
            new_field = varchar("phone", 30, default="blank"),
        ),
    ],
)
```

Apply:

```bash
morphic migrate showsql   # review the SQL
morphic migrate up
```

---

## Removing a Field

Remove the `notes` field from `orders`. Delete it from `schema/schema.star`, then generate:

```bash
morphic generate --name "remove_order_notes"
```

Because removing a column is destructive (data loss), morphic prompts you:

```
  Destructive operation detected: field_removed on "orders.notes"
  1) Generate  — include operation in migration
  2) Review    — include with // REVIEW comment
  3) Omit      — skip operation; schema state still advances (SchemaOnly)
  4) Exit      — cancel migration generation
  5) All       — generate all remaining destructive ops without prompting
Choice [1-5]: 1
```

Choose **1** to generate the drop, or **2** to flag it for peer review first. The generated operation:

```starlark
drop_field("orders", "notes")
```

Apply after review:

```bash
morphic migrate up
```

---

## Removing a Table

Remove `product_categories` from `schema/schema.star` entirely, then generate:

```bash
morphic generate --name "remove_product_categories"
```

Again you will be prompted for the destructive operation. The generated operation:

```starlark
drop_table("product_categories")
```

> **Warning:** This permanently drops the table and all its data. Verify with `morphic migrate showsql` before running `up`.

---

## Inserting Seed Data

Data migrations use `run_sql` or `upsert_data` and are written **by hand** — the diff engine only generates DDL operations. Create a new file in `migrations/`:

```starlark
# migrations/0008_seed_categories.star
migration(
    name = "0008_seed_categories",
    dependencies = ["0007_remove_product_categories"],
    operations = [
        run_sql(
            forward = """
INSERT INTO categories (name, slug, parent_id) VALUES
    ('Electronics',       'electronics',       NULL),
    ('Clothing',          'clothing',          NULL),
    ('Books',             'books',             NULL),
    ('Smartphones',       'smartphones',       1),
    ('Laptops',           'laptops',           1),
    ('Men''s Clothing',   'mens-clothing',     2),
    ('Women''s Clothing', 'womens-clothing',   2);
""",
            backward = """
DELETE FROM categories
WHERE slug IN (
    'electronics', 'clothing', 'books',
    'smartphones', 'laptops',
    'mens-clothing', 'womens-clothing'
);
""",
        ),
    ],
)
```

Apply:

```bash
morphic migrate up
```

> **Tip:** Keep schema changes (DDL) and data changes (DML) in separate migrations. This makes rollback cleaner and reduces lock contention on large tables.

---

## Adding a Stored Procedure

Stored procedures (and any other PostgreSQL-specific DDL like views, triggers, or custom functions) are added using `run_sql`. The `backward` should drop the procedure so rollback works cleanly.

```starlark
# migrations/0009_add_calculate_order_total_proc.star
migration(
    name = "0009_add_calculate_order_total_proc",
    dependencies = ["0008_seed_categories"],
    operations = [
        run_sql(
            forward = """
CREATE OR REPLACE FUNCTION calculate_order_total(p_user_id UUID)
RETURNS TABLE (
    order_id    UUID,
    placed_at   TIMESTAMP,
    item_count  BIGINT,
    total       DECIMAL(12,2)
)
LANGUAGE sql
STABLE
AS $$
    SELECT
        o.id                        AS order_id,
        o.placed_at,
        COUNT(*)                    AS item_count,
        SUM(p.price)                AS total
    FROM orders o
    JOIN products p
        ON p.id = ANY(
            -- placeholder join — replace with your actual order_items table
            ARRAY[]::UUID[]
        )
    WHERE o.user_id = p_user_id
    GROUP BY o.id, o.placed_at
    ORDER BY o.placed_at DESC;
$$;

COMMENT ON FUNCTION calculate_order_total(UUID)
    IS 'Returns a summary of all orders for a given user, including item count and total price.';
""",
            backward = "DROP FUNCTION IF EXISTS calculate_order_total(UUID);",
        ),
    ],
)
```

For a simpler example — a trigger that keeps `updated_at` current without relying on the application layer:

```starlark
# migrations/0010_add_updated_at_trigger.star
migration(
    name = "0010_add_updated_at_trigger",
    dependencies = ["0009_add_calculate_order_total_proc"],
    operations = [
        # Create the shared trigger function once
        run_sql(
            forward = """
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;
""",
            backward = "DROP FUNCTION IF EXISTS set_updated_at() CASCADE;",
        ),
        # Attach the trigger to the users table
        run_sql(
            forward = """
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
""",
            backward = "DROP TRIGGER IF EXISTS trg_users_updated_at ON users;",
        ),
        # Attach the trigger to the orders table
        run_sql(
            forward = """
CREATE TRIGGER trg_orders_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
""",
            backward = "DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;",
        ),
    ],
)
```

Apply:

```bash
morphic migrate showsql   # confirm the SQL looks right
morphic migrate up
```

---

## Rolling Back

Roll back the most recent migration:

```bash
morphic migrate down
```

Roll back the last three migrations:

```bash
morphic migrate down --steps 3
```

Roll back until (but not including) a specific migration:

```bash
morphic migrate down --to 0005_make_phone_required
```

Each operation's down path reverses the forward change automatically for typed operations (`create_table` → `DROP TABLE`, `add_field` → `DROP COLUMN`, etc.). `run_sql` operations use the `backward` SQL you provided.

---

## Day-to-Day Workflow Summary

```
Edit schema/schema.star
        │
        ▼
morphic generate --name "describe_the_change"
        │
        ▼
(optional) edit the generated .star file to add run_sql data steps
        │
        ▼
morphic migrate showsql          ← review SQL before touching the DB
        │
        ▼
morphic migrate up               ← apply
        │
        ▼
morphic migrate status           ← verify
```

### Useful Commands Reference

| Command | What it does |
|---------|-------------|
| `morphic init` | Bootstrap the `migrations/` directory |
| `morphic generate` | Generate a migration from schema changes |
| `morphic generate --dry-run` | Preview migration source without writing a file |
| `morphic generate --check` | CI mode: exit 1 if migrations are needed |
| `morphic generate --merge` | Generate a merge migration for concurrent branches |
| `morphic current-state` | Show reconstructed schema from migration DAG as Starlark |
| `morphic schema-to-sql` | Show full CREATE TABLE SQL from the Starlark schema |
| `morphic schema-to-sql --verbose` | Include processing detail in the output |
| `morphic migrate showsql` | Show SQL for all pending migrations |
| `morphic migrate up` | Apply all pending migrations |
| `morphic migrate up --to NAME` | Apply up to a named migration |
| `morphic migrate down` | Roll back the last applied migration |
| `morphic migrate down --steps N` | Roll back N migrations |
| `morphic migrate status` | Show applied vs pending migrations |
| `morphic migrate fake NAME` | Mark a migration applied without running SQL |
| `morphic migrate dag` | Print the migration dependency graph |

---

## See Also

- [Schema Format Reference](schema-format.md) — complete Starlark schema DSL syntax
- [Starlark Migrations Guide](starlark-migrations.md) — Starlark DSL reference and all operation types
- [init Command](commands/init.md) — detailed init options
- [morphic Command](commands/morphic.md) — all generation flags
- [migrate Command](commands/migrate.md) — all runtime commands and flags
- [diff Command](commands/diff.md) — compare schema against migration state
- [db-diff Command](commands/db-diff.md) — compare migration state against live database
- [current-state Command](commands/current-state.md) — inspect reconstructed migration state
- [schema-to-sql Command](commands/schema_to_sql.md) — schema inspection command
- [Configuration Guide](configuration.md) — full configuration reference
