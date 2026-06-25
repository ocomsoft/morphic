# morphic

A database migration tool with a Django-style workflow. Define your schema in Starlark, generate migration files, and run them in-process — no Go toolchain required at runtime, no compiled binary to ship.

> **How migrations run.** Generated migration files are `.star` scripts —
> a Python-like DSL powered by [Starlark-Go](https://pkg.go.dev/go.starlark.net).
> At runtime `morphic migrate` evaluates each `.star` file in-process
> and executes the migration operations against your database. The language
> is concise and readable, with typed builtins for common schema operations.

## Why morphic?

- **Readable schema & migrations**: Starlark is a Python-like language — schemas and migrations read like documentation
- **No build step at runtime**: Starlark files are interpreted directly; no `go build`, no temporary binary
- **Database-agnostic schema**: Write once, deploy to PostgreSQL, MySQL, SQLite, or SQL Server — symbolic defaults and type mappings resolve per-database at migration time
- **DAG-based ordering**: Migrations form a dependency graph so parallel branches merge cleanly
- **Auto change detection**: Diff schemas, generate only what changed
- **Safe destructive ops**: Field removals, table drops, and renames require explicit review

---

## Quick Start

### 1. Install

```bash
go install github.com/ocomsoft/morphic@latest
```

### 2. Initialise your project

```bash
cd your-project
morphic init
```

This creates:

```
your-project/
└── migrations/
    └── morphic.config.yaml   ← migration configuration
```

### 3. Define your schema

`schema/schema.star`:

```python
database("myapp", "1.0.0")

defaults("postgresql", {
    "new_uuid": "gen_random_uuid()",
    "now": "CURRENT_TIMESTAMP",
    "today": "CURRENT_DATE",
    "zero": "0",
    "false": "false",
    "true": "true",
    "blank": "''",
})

table("users",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        varchar("email", 255),
        varchar("name", 100, nullable=True),
        timestamp("created_at", auto_create=True),
    ],
    indexes = [
        index("users_email_idx", ["email"], unique=True),
    ],
)

table("posts",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        varchar("title", 200),
        text("body", nullable=True),
        timestamp("created_at", default="now"),
        foreign_key("user_id", fk("users", on_delete="CASCADE")),
    ],
)
```

The `defaults` function maps symbolic names (like `new_uuid`) to database-specific SQL expressions. Fields reference them by name — morphic resolves the correct expression for each target database at migration time.

### 4. Generate your first migration

```bash
morphic generate --name "initial"
# Creates: migrations/0001_initial.star
```

### 5. Apply to your database

```bash
export DATABASE_URL="postgresql://user:pass@localhost/mydb"
morphic migrate up
```

`morphic migrate` interprets the migration files in-process and runs the embedded migration App. No `go build`, no temporary binary.

---

## Day-to-Day Workflow

```bash
# 1. Edit your schema
vim schema/schema.star

# 2. Preview what will be generated
morphic generate --dry-run

# 3. Generate the migration
morphic generate --name "add user preferences"
# Creates: migrations/0004_add_user_preferences.star

# 4. Review the SQL before applying
morphic migrate showsql

# 5. Apply
morphic migrate up

# 6. Verify
morphic migrate status
```

---

## migrate Subcommands

`morphic migrate` interprets the migration files in-process and runs the embedded App. All arguments are forwarded:

```bash
morphic migrate up                       # apply all pending
morphic migrate up --to 0003_add_index   # apply up to a specific migration
morphic migrate down                     # roll back one
morphic migrate down --steps 3           # roll back multiple
morphic migrate status                   # show applied / pending
morphic migrate showsql                  # print SQL without running it
morphic migrate fake 0001_initial        # mark applied without running SQL
morphic migrate dag                      # show migration dependency graph
```

---

## Migration File Format

Migrations use Starlark with typed builtins for every common schema operation:

```python
migration(
    name = "0003_add_job",
    dependencies = ["0002_create_parts"],
    operations = [
        create_table("job",
            fields = [
                uuid("id", primary_key=True, default="new_uuid"),
                varchar("title", 120),
                text("description", nullable=True),
                integer("priority", default="zero"),
                timestamp("created_date", default="now"),
                foreign_key("contact_id", fk("contact", on_delete="CASCADE")),
            ],
            indexes = [
                index("job_status_idx", ["status"]),
            ],
        ),
    ],
)
```

See the [Starlark Migration Format](docs/starlark-migrations.md) for the full DSL reference including all field types, operations, and patterns.

---

## Project Structure

```
your-project/
├── schema/
│   └── schema.star               ← your Starlark schema definition
├── migrations/
│   ├── morphic.config.yaml       ← migration configuration
│   ├── 0001_initial.star         ← migration files (generated by morphic)
│   ├── 0002_add_posts.star
│   └── 0003_add_index.star
├── go.mod
└── main.go
```

---

## Database Support

| Database | Status | Notes |
|----------|--------|-------|
| **PostgreSQL** | Full | UUID, JSONB, arrays, advanced types |
| **MySQL** | Supported | JSON, AUTO_INCREMENT, InnoDB |
| **SQLite** | Supported | Simplified types, basic constraints |
| **SQL Server** | Supported | UNIQUEIDENTIFIER, NVARCHAR, BIT |
| **Amazon Redshift** | Provider ready | SUPER JSON, IDENTITY sequences |
| **ClickHouse** | Provider ready | MergeTree engine, Nullable types |
| **TiDB** | Provider ready | MySQL-compatible, distributed |
| **Vertica** | Provider ready | Columnar analytics |
| **YDB (Yandex)** | Provider ready | Optional<Type>, native JSON |
| **Turso** | Provider ready | Edge SQLite |
| **StarRocks** | Provider ready | MPP analytics, OLAP |
| **Aurora DSQL** | Provider ready | AWS serverless, PostgreSQL-compatible |

> PostgreSQL has been tested against real database instances. All other providers have comprehensive unit tests but may need additional validation for production.

---

## Destructive Operations

When a field removal, table drop, or rename is detected, morphic shows an interactive prompt before generating:

```
⚠  Destructive: field_removed on "users" (field: "legacy_col")
SQL: ALTER TABLE "users" DROP COLUMN "legacy_col"

> Generate       — include operation in migration
  Review         — include with // REVIEW comment
  Omit           — skip SQL; schema state still advances (SchemaOnly)
  IgnoreErrors   — include with IgnoreErrors: true
  Exit           — cancel migration generation

  Scope: [This only]  All remaining   All of this type

↑/↓ select • tab scope • enter confirm • esc cancel
```

Use **Tab** to cycle the scope — *This only*, *All remaining* (apply your choice to every subsequent destructive op), or *All of this type* (e.g. apply to all `field_removed` ops but still prompt for `table_removed`).

---

## Branch & Merge

When two developers generate migrations concurrently the DAG develops branches:

```
0001_initial
├── 0002_add_messaging   (developer A)
└── 0003_add_payments    (developer B)
```

Resolve with a merge migration:

```bash
morphic generate --merge
# Creates: migrations/0004_merge_0002_add_messaging_and_0003_add_payments.star
```

---

## Configuration

### Database connection

`morphic migrate` reads connection details from `DATABASE_URL` and `DB_TYPE`:

```bash
export DATABASE_URL="postgresql://user:pass@localhost/mydb"
export DB_TYPE=postgresql   # optional, defaults to postgresql
```

### Configuration file

`migrations/morphic.config.yaml`:

```yaml
database:
  type: postgresql

migration:
  directory: migrations

output:
  verbose: false
```

See the [Configuration Guide](docs/configuration.md) for complete options.

---

## Documentation

### Guides

- **[Installation Guide](docs/installation.md)**
- **[Schema Format Guide](docs/schema-format.md)** — schema reference
- **[Starlark Migration Format](docs/starlark-migrations.md)** — full DSL reference for migration files
- **[Configuration Guide](docs/configuration.md)**
- **[Claude Code Skill](docs/claude-code-skill.md)** — auto-triggered AI assistant skill for schema-first migrations

### Command Reference

**Schema & Generation**

| Command | Description |
|---------|-------------|
| **[init](docs/commands/init.md)** | Initialize migrations directory and config |
| **[generate](docs/commands/generate.md)** | Generate migration files from schema changes |
| [generate empty](docs/commands/empty.md) | Create a blank migration for custom operations |
| [generate dump-data](docs/commands/dump-data.md) | Generate a data-seeding migration from live DB |

**Migration Runtime**

| Command | Description |
|---------|-------------|
| **[migrate](docs/commands/migrate.md)** | Run migrations in-process via Starlark interpreter |

**Inspection & Debugging**

| Command | Description |
|---------|-------------|
| [schema-diff](docs/commands/schema_diff.md) | Show drift between schema and migration state |
| [db-diff](docs/commands/db-diff.md) | Compare live DB schema against migration state |
| [current-state](docs/commands/current_state.md) | Show reconstructed schema state |
| [schema-to-sql](docs/commands/schema_to_sql.md) | Convert merged schema to SQL |
| [schema-to-diagram](docs/commands/schema2diagram.md) | Generate Markdown docs with diagrams |
| [version](docs/commands/version.md) | Show version and build info |

**Conversion Tools**

| Command | Description |
|---------|-------------|
| [db-to-schema](docs/commands/db2schema.md) | Reverse-engineer schema from existing DB |
| [struct-to-schema](docs/commands/struct2schema.md) | Convert Go structs to schema |
| [find-includes](docs/commands/find_includes.md) | Discover schema includes from Go modules |
| [from-makemigrations](docs/commands/convert.md) | Convert Go migrations to Starlark format |
| [yaml2dsl](docs/commands/yaml2dsl.md) | Convert YAML schema to Starlark DSL |

---

## Claude Code Skill

morphic includes a **Claude Code skill** that auto-triggers whenever Claude detects database schema work in your Go project. It enforces the correct workflow: edit `schema/schema.star` → generate migrations → avoid raw SQL.

### Installing the Skill

**Option 1: Personal skill** (recommended — available in all Go projects):

```bash
cp -r skills/ ~/.claude/skills/go-morphic
```

**Option 2: Plugin** (if the repo is registered as a Claude Code plugin):

The `.claude-plugin/` directory is detected automatically when installed.

### What the Skill Does

- Auto-triggers when you ask Claude to add tables, fields, indexes, foreign keys, or modify columns
- Guides Claude through init → schema edit → migration generation → SQL preview → verification
- Provides quick reference for field types, properties, indexes, defaults, and type mappings
- Enforces schema-first workflow — RunSQL only as a last resort
- Covers both new project bootstrapping and ongoing schema changes

See [`skills/SKILL.md`](skills/SKILL.md) for the full skill content and [`docs/claude-code-skill.md`](docs/claude-code-skill.md) for detailed usage.

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Add tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Submit a pull request

## License

MIT License — see [LICENSE](LICENSE) file for details.

---

**Ready to get started?** Run `morphic init` in your project directory.
