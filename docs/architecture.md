# Architecture Documentation

## Overview

Morphic is a Starlark-based database migration tool for Go that generates typed migration source files from declarative schema definitions. It follows a Django-inspired workflow: schema definitions and migrations are written in [Starlark](https://github.com/google/starlark-go) (a deterministic Python-like language) and evaluated at runtime by the embedded Starlark-Go interpreter — no compilation step required.

> **Runtime model.** Migrations are `.star` files evaluated at runtime by the
> [Starlark-Go](https://github.com/google/starlark-go) interpreter embedded in
> the morphic CLI. Each `.star` file calls `migration()` at the top level to
> register itself. Starlark is a deterministic, sandboxed scripting language
> with Python-like syntax; it has no I/O side effects by default, which makes
> migration files safe to evaluate repeatedly for schema-state reconstruction.

The tool supports two distinct workflows:

- **Starlark migrations (primary)** — Generates `.star` migration files evaluated via the Starlark-Go interpreter. The CLI's in-process registry is the single source of truth for migration state. No separate state file is required.
- **SQL migrations (legacy, opt-in via `--sql`)** — Generates Goose-compatible `.sql` files applied via `morphic goose up`.

---

## System Architecture

### High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        Developer Workflow                         │
│                                                                   │
│   schema/schema.star  ──►  morphic generate         │
│                                    │                             │
│                         migrations/0001_initial.star             │
│                         migrations/0002_add_users.star  ...      │
│                                    │                             │
│         morphic migrate up | down | status                │
│         (Starlark-Go interprets the .star files in-process)      │
└──────────────────────────────────────────────────────────────────┘
                                    │
┌──────────────────────────────────────────────────────────────────┐
│                     morphic CLI (cmd/)                     │
│                                                                   │
│  ┌────────────────┐  ┌──────────────────┐  ┌─────────────────┐  │
│  │  morphic│  │  init / go-init  │  │  sql-migrations │  │
│  │ (Starlark gen) │  │  (bootstrapper)  │  │  (legacy SQL)   │  │
│  └────────────────┘  └──────────────────┘  └─────────────────┘  │
│                                                                   │
│  ┌────────────────┐  ┌──────────────────┐  ┌─────────────────┐  │
│  │  db2schema     │  │  struct2schema   │  │  schema2diagram │  │
│  │  (introspect)  │  │  (Go → schema)   │  │  (visualise)    │  │
│  └────────────────┘  └──────────────────┘  └─────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
                                    │
┌──────────────────────────────────────────────────────────────────┐
│                     Core Processing Layer                         │
│                                                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐            │
│  │Schema Parser│  │ Diff Engine │  │  Starlark    │            │
│  │(schema.star │  │ (SchemaDiff)│  │  Codegen     │            │
│  │  → Schema)  │  │             │  │  (StarlarkGen│            │
│  └─────────────┘  └─────────────┘  └──────────────┘            │
│                                                                   │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────┐            │
│  │   Registry  │  │    Graph    │  │ SchemaState  │            │
│  │ (migration()│  │  (DAG +     │  │  (in-memory  │            │
│  │  top-level) │  │   Kahn's)   │  │   replay)    │            │
│  └─────────────┘  └─────────────┘  └──────────────┘            │
└──────────────────────────────────────────────────────────────────┘
                                    │
┌──────────────────────────────────────────────────────────────────┐
│                   Database Provider Layer (internal/providers/)   │
│                                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │PostgreSQL│  │  MySQL   │  │  SQLite  │  │SQLServer │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
│                                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │ Redshift │  │ClickHouse│  │   TiDB   │  │ Vertica  │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
│                                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │   YDB    │  │  Turso   │  │StarRocks │  │AuroraDSQL│        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└──────────────────────────────────────────────────────────────────┘
                                    │
┌──────────────────────────────────────────────────────────────────┐
│              Embedded Migration Runtime (migrate/)                │
│                                                                   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │  Runner  │  │ Recorder │  │   App    │  │Starlark  │        │
│  │  Up/Down │  │ history  │  │ (CLI in  │  │ loader   │        │
│  │  ShowSQL │  │  table   │  │  process)│  │ (interp) │        │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘        │
└──────────────────────────────────────────────────────────────────┘
```

---

## Primary Workflow: Starlark Migrations

### End-to-End Developer Flow

```
1. Developer edits schema/schema.star
         │
2. morphic generate
   ├── Parses schema/schema.star  (internal/starlark_loader)
   ├── Loads existing migrations/*.star via Starlark-Go (internal/interp.LoadRegistry)
   │        └── Builds DAG and replays SchemaState in-process — no compilation
   ├── Diffs schema against reconstructed SchemaState
   │        └── internal/yaml: SchemaDiff
   └── Writes migrations/NNNN_<name>.star  (internal/codegen: StarlarkGenerator)
         │
3. Developer applies
   ├── morphic migrate up          (apply pending)
   ├── morphic migrate down        (rollback)
   ├── morphic migrate status      (show applied/pending)
   ├── morphic migrate showsql     (preview SQL without executing)
   └── morphic migrate dag         (visualise DAG)

   In each case the CLI loads migrations/*.star via the Starlark-Go interpreter,
   then invokes migrate.NewAppWithRegistry(...).Run(args) directly — no fork/exec.
```

### Branch Detection and Merge Migrations

When two developers independently create migrations from the same parent, the graph has multiple leaf nodes. The `morphic generate --merge` flag generates a merge migration:

```
0001_initial ──► 0002_add_users   (developer A)
             └─► 0002_add_posts   (developer B)
                     │
             morphic generate --merge
                     │
             0003_merge.star  (Dependencies: ["0002_add_users", "0002_add_posts"], Operations: [])
```

The merge migration has two parents and empty operations. It exists solely to re-linearise the graph.

### Squash Migrations

Old migrations can be collapsed into a single squash migration using `SquashGenerator` (`internal/codegen/squash_generator.go`). The resulting migration carries a `Replaces` field listing the names of all migrations it supersedes. The Runner skips individual migrations when their names appear in a `Replaces` list.

---

## Legacy Workflow: SQL Migrations

The SQL workflow is opt-in and retained for compatibility. It uses YAML snapshots and generates Goose-compatible `.sql` files.

```
1. morphic init --sql
   └── Creates migrations/ with .schema_snapshot.yaml
         │
2. Developer edits schema/schema.star
         │
3. morphic sql-migrations
   ├── Diffs schema against .schema_snapshot.yaml
   └── Writes migrations/NNNN_<name>.sql (-- +goose Up / +goose Down)
         │
4. morphic goose up
   └── Delegates to the Goose migration runner
```

Key differences from the Starlark workflow:

| Concern              | Starlark migrations (primary)              | SQL migrations (legacy)            |
|----------------------|--------------------------------------------|-------------------------------------|
| State storage        | Starlark-loaded registry (DAG replay)      | `.schema_snapshot.yaml` file        |
| Migration format     | `.star` (typed, interpreted at runtime)    | `.sql` (Goose format)               |
| Execution            | `morphic migrate up` (in-proc)             | `morphic goose up`                  |
| Branch detection     | Graph leaves, `--merge` flag               | Not supported                       |
| VCS merge conflicts  | None (graph reloaded from .star files)     | Possible (snapshot file)            |

---

## Core Components

### 1. Command Layer (`cmd/`)

Each command is in its own source file. The CLI is built with Cobra.

| File                   | Command                        | Purpose                                                     |
|------------------------|--------------------------------|-------------------------------------------------------------|
| `root.go`              | `morphic`               | Root command, Viper config loading                          |
| `go_migrations.go`     | `morphic generate`| Starlark migration generator (primary workflow)             |
| `go_init.go`           | `morphic init`          | Bootstrap `migrations/` directory structure                 |
| `sql_migrations.go`    | `morphic sql-migrations`| Legacy SQL migration generator                              |
| `init_sql.go`          | `morphic init --sql`    | Legacy SQL project setup                                    |
| `goose.go`             | `morphic goose`         | Goose runner integration (legacy)                           |
| `db2schema.go`         | `morphic db2schema`     | Reverse-engineer DB to schema                               |
| `struct2schema.go`     | `morphic struct2schema` | Convert Go structs to schema                                |
| `schema2diagram.go`    | `morphic schema2diagram`| Visualise schema as diagram                                 |
| `schema_to_sql.go`          | `morphic schema-to-sql`      | Generate SQL without writing migration files           |
| `find_includes.go`     | `morphic find-includes` | Discover schema includes from Go modules                    |

### 2. `migrate/` Package (Runtime Library)

This is the library used by all generated migration files. It is self-contained and has no dependency on the `cmd/` or `internal/` packages — keeping the public API surface tight makes it straightforward to expose its types as Starlark built-ins via the Starlark-Go loader.

#### 2.1 Type System (`migrate/types.go`)

```go
// Migration is a single migration node in the DAG.
type Migration struct {
    Name         string      // Unique identifier e.g. "0001_initial"
    Dependencies []string    // Parent migration names
    Operations   []Operation // Schema changes applied in order
    Replaces     []string    // For squash migrations: names of replaced migrations
}

// Field is a database column definition used in operations.
type Field struct {
    Name       string
    Type       string      // varchar, text, integer, uuid, boolean, timestamp, ...
    PrimaryKey bool
    Nullable   bool
    Default    string
    Length     int
    Precision  int
    Scale      int
    AutoCreate bool
    AutoUpdate bool
    ForeignKey *ForeignKey
    ManyToMany *ManyToMany
}

type ForeignKey struct {
    Table    string
    OnDelete string
    OnUpdate string
}

type ManyToMany struct {
    Table string
}

type Index struct {
    Name   string
    Fields []string
    Unique bool
}
```

#### 2.2 Operation Interface and Concrete Types (`migrate/operations.go`)

```go
type Operation interface {
    TypeName() string           // e.g. "CreateTable"
    TableName() string          // primary table affected
    Describe() string           // human-readable description
    ForwardSQL(provider) string // SQL to apply
    ReverseSQL(provider) string // SQL to rollback
    Mutate(*SchemaState) error  // mutates in-memory state
}
```

The 10 concrete operation types are:

| Type            | Description                              |
|-----------------|------------------------------------------|
| `CreateTable`   | CREATE TABLE with fields and indexes     |
| `DropTable`     | DROP TABLE                               |
| `RenameTable`   | ALTER TABLE ... RENAME TO ...            |
| `AddField`      | ALTER TABLE ... ADD COLUMN ...           |
| `DropField`     | ALTER TABLE ... DROP COLUMN ...          |
| `AlterField`    | ALTER TABLE ... ALTER COLUMN ...         |
| `RenameField`   | ALTER TABLE ... RENAME COLUMN ...        |
| `AddIndex`      | CREATE [UNIQUE] INDEX ...                |
| `DropIndex`     | DROP INDEX ...                           |
| `RunSQL`        | Arbitrary SQL (forward + reverse pair)   |

#### 2.3 Registry (`migrate/registry.go`)

The Registry is populated when the Starlark-Go loader evaluates each `.star` file. Each generated `.star` file calls the `migration()` built-in at the top level; the loader maps this call to `Registry.Register()` on a fresh per-load registry. There is no `init()` or `main()` involvement — Starlark files are pure data declarations.

```go
// Global registry used by App when running as a standalone tool.
var globalRegistry = NewRegistry()

// Register adds a migration. Panics on nil or duplicate name.
func Register(m *Migration) { globalRegistry.Register(m) }

// GlobalRegistry is used by App to build the Graph.
func GlobalRegistry() *Registry { return globalRegistry }
```

The `Registry` struct holds a `map[string]*Migration` plus an insertion-order slice. It is protected by a `sync.RWMutex` for safe concurrent use.

#### 2.4 Graph and DAG (`migrate/graph.go`)

`Graph` is a directed acyclic graph where each node is a `Migration` and each directed edge represents a dependency.

```
BuildGraph(registry)
    └── wires parent/child pointers from Migration.Dependencies
    └── calls detectCycles() (DFS white/grey/black colouring)

Graph.Linearize()
    └── Kahn's algorithm with alphabetical tie-breaking for determinism
    └── Returns []*Migration in topological order

Graph.ReconstructState()
    └── Calls Linearize(), then replays all Operation.Mutate() calls
    └── Returns *SchemaState representing the full current schema

Graph.ToDAGOutput()
    └── Produces DAGOutput (JSON-serialisable) including SchemaState
    └── Returned in-process by queryDAG (cmd/go_migrations.go)
    └── Also emitted as JSON by `migrate dag --format json` for CLI inspection

Graph.DetectBranches()
    └── Returns leaf groups when multiple leaves exist (concurrent branches)
```

`DAGOutput` is how `morphic generate` reads the current schema state. With the Starlark-Go loader, the round-trip is direct: the CLI calls `interp.LoadRegistry` → `BuildGraph` → `ToDAGOutput` in the same process. The on-disk JSON format is preserved so external tooling that scrapes `migrate dag --format json` continues to work.

#### 2.5 SchemaState (`migrate/state.go`)

`SchemaState` is the in-memory representation of the database schema at any point in the graph. It eliminates the need for a separate snapshot file in the Go workflow.

```go
type SchemaState struct {
    Tables map[string]*TableState
}

type TableState struct {
    Name    string
    Fields  []Field
    Indexes []Index
}
```

Mutation methods: `AddTable`, `DropTable`, `RenameTable`, `AddField`, `DropField`, `AlterField`, `RenameField`, `AddIndex`, `DropIndex`. Each returns an error if the precondition is violated (e.g. adding a field to a non-existent table).

#### 2.6 Runner (`migrate/runner.go`)

`Runner` executes migrations against a live database. It receives a `*Graph`, a `providers.Provider`, a `*sql.DB`, and a `*MigrationRecorder`.

- `Up(to string)` — Linearises the graph, skips applied migrations (queried from `morphic_history`), applies each pending migration in a transaction, records it.
- `Down(steps int, to string)` — Rolls back in reverse topological order.
- `Status()` — Prints applied/pending status for each migration.
- `ShowSQL()` — Prints SQL for pending migrations without executing.

#### 2.7 MigrationRecorder (`migrate/recorder.go`)

Manages the `morphic_history` table in the target database:

```sql
CREATE TABLE IF NOT EXISTS morphic_history (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    applied_at TEXT DEFAULT CURRENT_TIMESTAMP
)
```

Methods: `EnsureTable`, `GetApplied`, `RecordApplied`, `RecordRolledBack`, `Fake`.

#### 2.8 App (`migrate/app.go`)

`App` is the embedded Cobra CLI for migration management. It is constructed in two places:

1. **Inside the morphic CLI** (primary path) — `cmd/migrate.go`'s `ExecuteMigrate` calls `migrate.NewAppWithRegistry(cfg, reg)` after `internal/interp.LoadRegistry` has populated `reg` from the migration `.star` files via the Starlark-Go interpreter. The App runs entirely in the morphic process.
2. **Inside the optional standalone binary** — the entry-point template generated by `morphic init` calls `migrate.NewApp(cfg)`, which uses the package-level `globalRegistry`.

Commands exposed by `App`:

| Subcommand       | Description                                          |
|------------------|------------------------------------------------------|
| `up [--to NAME]` | Apply pending migrations                             |
| `down [--steps N] [--to NAME]` | Rollback migrations                   |
| `status`         | Show applied/pending status                          |
| `showsql`        | Print SQL for pending migrations                     |
| `fake NAME`      | Mark a migration applied without executing SQL       |
| `dag [--format ascii\|json]` | Visualise or export the migration graph  |

#### 2.9 Starlark Built-ins (`internal/starlark_loader/`)

The Starlark-Go loader exposes `migrate` package types directly as Starlark built-in functions and values. There is no separate symbol-map extraction step. The loader defines built-ins such as `migration()`, `create_table()`, `add_field()`, etc., which construct the corresponding Go `migrate.*` types and register them into the per-load `*migrate.Registry`. This approach is simpler than the previous yaegi symbol map: new built-ins are added by registering additional `starlark.Builtin` values in the loader, with no code-generation step.

### 3. Code Generation (`internal/codegen/`)

#### 3.1 StarlarkGenerator (`internal/codegen/starlark_generator.go`)

The primary generator. Converts a `yaml.SchemaDiff` into a `.star` migration file evaluated by the Starlark-Go interpreter. Output is human-readable Starlark with Python-like syntax.

```go
type StarlarkGenerator struct{}

// GenerateMigration produces a .star file for a single migration.
// The file calls migration() at the top level with name, dependencies,
// and a list of operation calls such as create_table(), add_field(), etc.
func (g *StarlarkGenerator) GenerateMigration(
    name string,
    dependencies []string,
    diff *yaml.SchemaDiff,
    currentSchema *yaml.Schema,
    previousSchema *yaml.Schema,
) ([]byte, error)
```

Example of a generated migration file:

```python
# 0001_initial.star
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        create_table(
            name = "users",
            fields = [
                field(name = "id",    type = "integer", primary_key = True),
                field(name = "email", type = "varchar", length = 255),
            ],
            indexes = [],
        ),
    ],
)
```

#### 3.1a GoGenerator (`internal/codegen/go_generator.go`)

The GoGenerator remains available for projects that opted into the Go format before the Starlark migration. It converts a `yaml.SchemaDiff` into a `.go` source file. The StarlarkGenerator is now the primary generator; GoGenerator is retained for backwards compatibility.

#### 3.2 MergeGenerator (`internal/codegen/merge_generator.go`)

Generates a merge migration when `--merge` is passed. The generated `.star` file has two or more entries in `dependencies` and an empty `operations` list.

#### 3.3 SquashGenerator (`internal/codegen/squash_generator.go`)

Collapses a range of migrations into a single squash migration. The result populates `Replaces` with the names of all squashed migrations. The Runner treats any migration whose name appears in `Replaces` as superseded.

### 4. Schema Processing (`internal/yaml/`)

Responsible for reading schema files and computing diffs. Although the package is named `yaml`, it handles both the Starlark schema format (`schema.star`) and the legacy YAML snapshot format used by the SQL workflow.

| File                      | Responsibility                                          |
|---------------------------|---------------------------------------------------------|
| `parser.go`               | Unmarshal YAML into `Schema` structs                    |
| `diff.go`                 | Compare two `Schema` values → `SchemaDiff`              |
| `types.go`                | Re-exports `internal/types` for YAML package consumers  |
| `state.go`                | `StateManager` — loads/saves `.schema_snapshot.yaml`    |
| `merger.go`               | Merges multiple schema sources                          |
| `include_processor.go`    | Resolves `include:` directives from Go modules          |
| `module_resolver.go`      | Locates Go module roots for include resolution          |
| `migration_generator.go`  | Legacy SQL generation from `SchemaDiff`                 |
| `header.go`               | Chain metadata read/write (legacy chain workflow)       |
| `chain.go`                | Chain traversal and fork detection (legacy)             |

The `SchemaDiff` type describes the delta between two schemas:

```go
type SchemaDiff struct {
    AddedTables    []Table
    RemovedTables  []Table
    ModifiedTables []TableDiff
}

type TableDiff struct {
    TableName    string
    AddedFields  []Field
    RemovedFields []Field
    AlteredFields []FieldDiff
    AddedIndexes  []Index
    RemovedIndexes []Index
}
```

### 5. Configuration System (`internal/config/`)

Configuration is loaded via Viper with the following priority (highest to lowest):

1. Command-line flags
2. Environment variables (`MORPHIC_*`)
3. Config file (`migrations/morphic.config.yaml`)
4. Default values

Key config fields:

```go
type MigrationConfig struct {
    Directory           string // default: "migrations"
    DatabaseType        string // postgresql, mysql, sqlite, etc.
    EnableChainMetadata bool   // enables chain metadata in SQL headers (legacy)
}
```

### 6. Database Provider System (`internal/providers/`)

All 12 providers implement a common interface used by the Runner to generate database-specific DDL at migration apply time.

```go
type Provider interface {
    GenerateCreateTable(table *TableState) (string, error)
    GenerateDropTable(tableName string) string
    GenerateAddColumn(tableName string, field *Field) string
    GenerateDropColumn(tableName string, fieldName string) string
    GenerateAlterColumn(tableName string, oldField, newField *Field) (string, error)
    GenerateRenameTable(oldName, newName string) string
    GenerateRenameColumn(tableName, oldName, newName string) string
    GenerateCreateIndex(tableName string, index *Index) string
    GenerateDropIndex(indexName string) string
    QuoteName(name string) string
    ConvertFieldType(field *Field) string
}
```

Provider factory:

```go
func NewProvider(dbType string) (Provider, error)
```

Supported databases: PostgreSQL, MySQL, SQLite, SQL Server, Redshift, ClickHouse, TiDB, Vertica, YDB, Turso, StarRocks, AuroraDSQL.

### 7. Type System (`internal/types/`)

Central schema type definitions shared across `internal/yaml/`, `internal/codegen/`, and `cmd/`:

```go
type Schema struct {
    Database DatabaseConfig
    Include  []Include   // External schema imports from Go modules
    Defaults Defaults    // Database-specific field defaults
    Tables   []Table
}

type Table struct {
    Name    string
    Fields  []Field
    Indexes []Index
}

type Field struct {
    Name       string
    Type       string
    PrimaryKey bool
    Nullable   *bool
    Default    string
    ForeignKey *ForeignKey
    // ... additional column properties
}
```

`DatabaseType` is a string type alias, not an enum, allowing custom database names.

### 8. Specialised Utilities

#### Struct2Schema (`internal/struct2schema/`)

AST-based conversion of Go structs to schema:

- Parses Go source files using `go/ast`
- Interprets struct tags: `db`, `gorm`, `sql`, `bun`
- Maps Go types to schema field types
- Detects foreign key relationships via tag conventions

#### DB2Schema (`cmd/db2schema.go`)

Reverse-engineers a live database to a schema file. Supports all 12 providers. Useful for bootstrapping schema files from an existing database.

---

## Design Patterns

### 1. Top-Level migration() Registration Pattern

The fundamental pattern for Starlark migrations. Each generated `.star` file in the `migrations/` directory calls the `migration()` built-in at the top level. When the Starlark-Go interpreter evaluates the file, this call registers the migration into the per-load `*migrate.Registry` provided by the loader. No `init()`, no `main()`, and no shimming are required — the built-in is injected directly by `internal/interp.LoadRegistry` before evaluation begins.

### 2. DAG as Single Source of Truth

The `Graph` built from the `Registry` is the authoritative record of migration history and schema state. There is no separate snapshot file in the Starlark workflow. `morphic generate` reads the current state by calling `interp.LoadRegistry` + `BuildGraph` + `ToDAGOutput` directly — no external process, no JSON serialisation round-trip.

### 3. State Reconstruction by Replay

`SchemaState` is rebuilt by replaying all operations in topological order (`Graph.ReconstructState()`). This is the same approach used by the Runner during `up` to avoid re-running already-applied migrations, and by the `morphic` generator to determine what the current schema looks like.

### 4. Provider Strategy Pattern

Each database provider implements the same `Provider` interface. The Runner receives a provider at construction time and delegates all SQL generation to it. This isolates database-specific logic and allows mock providers in tests.

### 5. Command Pattern

Each CLI command is its own source file in `cmd/`. Each command is a Cobra `*cobra.Command` registered on the root. Business logic is implemented in `internal/` packages; commands are thin wrappers.

### 6. Factory Pattern

`NewProvider(dbType string)` and `NewApp(cfg Config)` are the primary factory functions. They centralise construction decisions and prevent the rest of the codebase from importing provider implementations directly.

---

## Data Flow

### Starlark Migration Generation Flow

```
1. Developer edits schema/schema.star
         │
2. cmd/go_migrations.go
   │
   ├── internal/starlark_loader: Parse schema/schema.star → Schema
   │
   ├── cmd/go_migrations.go: queryDAG(migrationsDir)
   │       │
   │       ├── internal/interp: LoadRegistry(migrationsDir)
   │       │       └── Starlark-Go evaluates each .star file → *migrate.Registry
   │       │
   │       └── migrate/graph.go: BuildGraph(reg).ToDAGOutput()
   │               └── Includes SchemaState (fully reconstructed)
   │
   ├── internal/yaml: Convert SchemaState → Schema (for diffing)
   │
   ├── internal/yaml/diff.go: Diff(previousSchema, currentSchema) → SchemaDiff
   │
   ├── internal/codegen/starlark_generator.go: GenerateMigration(name, deps, diff) → []byte
   │
   └── Write migrations/NNNN_<name>.star
```

### Migration Apply Flow (in-process via Starlark-Go)

```
1. Developer runs `morphic migrate up`
         │
2. cmd/migrate.go: ExecuteMigrate(migrationsDir, args)
   │
   ├── internal/interp.LoadRegistry(migrationsDir)
   │       └── Starlark-Go evaluates each .star file → *migrate.Registry
   │
   ├── migrate.NewAppWithRegistry(cfg, reg)
   │
   └── app.Run(args)
           │
           └── migrate/app.go: buildRunner()
                   │
                   ├── migrate/graph.go: BuildGraph(reg)
                   │       └── Validates all dependencies exist, no cycles
                   │
                   ├── migrate/recorder.go: GetApplied() → map[string]bool
                   │
                   └── migrate/runner.go: Up("")
                           │
                           ├── graph.Linearize() → []*Migration (topological order)
                           ├── Replay already-applied migrations → SchemaState
                           │
                           └── For each pending migration:
                                   ├── op.ForwardSQL(provider) → SQL string
                                   ├── db.Exec(SQL)
                                   ├── op.Mutate(state) → update in-memory state
                                   └── recorder.RecordApplied(name)
```

### Struct2Schema Flow

```
1. cmd/struct2schema.go
         │
2. internal/struct2schema: Parse Go source files via go/ast
         │
3. Extract struct definitions + tags
         │
4. Map Go types → schema field types
         │
5. Detect relationships (foreign keys via tags)
         │
6. internal/yaml: Write schema.star (or schema.yaml for legacy SQL workflow)
```

---

## Error Handling Strategy

### Validation Layers

1. **Starlark Syntax** — Caught by the Starlark-Go parser during schema or migration loading; reported with file and line context.
2. **Schema Semantics** — Type checking, required fields, constraint validation in `internal/yaml` and `internal/starlark_loader`.
3. **Graph Integrity** — Missing dependencies and cycles detected in `migrate/graph.go` before any SQL is generated or executed.
4. **Operation Preconditions** — `SchemaState` mutation methods return errors for violated preconditions (duplicate table, missing field, etc.).
5. **Runtime** — Database connectivity, permission errors, and SQL execution failures are wrapped and returned from `Runner`.

### Error Categories

- **Fatal** — Invalid config, unparseable `.star` file, graph cycle. Execution stops immediately.
- **Graph errors** — Missing dependency, duplicate migration name. The loader panics on duplicate registration (caught at load time).
- **Validation errors** — Collected and reported in full before stopping.
- **Warnings** — Destructive operations (DropTable, DropField) are logged prominently and flagged in generated migration files.

---

## Security Considerations

1. **No direct SQL execution by the generator** — `morphic generate` only writes `.star` files. SQL is only executed when the developer explicitly runs `morphic migrate up` (or the optional standalone binary).
2. **Input validation** — Strict schema validation before any processing.
3. **SQL injection prevention** — Identifier quoting via `provider.QuoteName()` throughout all DDL generation.
4. **No credential storage** — Database credentials are passed via environment variables or DSN at runtime by the developer; never stored in generated files or config committed to VCS.
5. **Destructive operations are explicit** — `DropTable`, `DropField`, and `AlterField` (type changes) are always visible in the generated `.star` file and reviewed before the migrations are applied.
6. **Interpreted code runs in the morphic process** — Starlark-loaded migrations execute with the same OS-level privileges as the CLI itself. Starlark is sandboxed by default (no I/O, no network, no `import`), which limits the blast radius of malicious or buggy `.star` files compared to general-purpose interpreted code. Treat the `migrations/` directory as source-controlled configuration: review changes before running them.

---

## Extension Points

### Adding a New Database Provider

1. Implement `internal/providers.Provider`.
2. Add a case to the `NewProvider` factory function.
3. Add type mapping constants.
4. Write provider-specific DDL tests.
5. Document any limitations (e.g. unsupported DDL operations) as comments.

### Adding a New Operation Type

1. Add the struct to `migrate/operations.go` implementing `Operation`.
2. Add a `Mutate(*SchemaState)` implementation.
3. Implement `ForwardSQL` and `ReverseSQL` for all providers.
4. Handle the new type in `internal/codegen/starlark_generator.go` (code emission) and register a corresponding built-in in `internal/starlark_loader`.
5. Add a case to `internal/yaml/diff.go` (diff detection).

### Adding a New CLI Command

1. Create a new file in `cmd/` (one command per file).
2. Register the command on the root in `cmd/root.go`.
3. Implement business logic in the relevant `internal/` package.
4. Add documentation in `docs/commands/`.

---

## Testing Strategy

### Test Levels

1. **Unit tests** — Component-level; each package has `_test.go` files. Registry, Graph, SchemaState, and all Operation types have table-driven unit tests.
2. **Integration tests** — `integration_test.go` at the project root and `yaml_integration_test.go` exercise full pipelines: parse schema → diff → generate → load via Starlark-Go → run.
3. **Provider tests** — Each provider is tested for correct DDL output for all 10 operation types.
4. **End-to-end tests** — Full command execution via `go test ./...` invoking CLI commands and verifying file output.

### Key Test Files

| File                                                    | Coverage area                                         |
|---------------------------------------------------------|-------------------------------------------------------|
| `migrate/registry_test.go`                              | Registration, panic on duplicate/nil                  |
| `migrate/graph_test.go`                                 | Topological sort, cycle detection, branches           |
| `migrate/state_test.go`                                 | SchemaState mutation correctness                      |
| `migrate/operations_test.go`                            | All 10 operation types, forward/reverse SQL           |
| `migrate/runner_test.go`                                | Up/Down/Status/ShowSQL                                |
| `internal/codegen/starlark_generator_test.go`           | Generated Starlark source correctness                 |
| `internal/codegen/go_generator_test.go`                 | Generated Go source correctness (legacy format)       |
| `internal/codegen/merge_generator_test.go`              | Merge migration generation                            |
| `internal/codegen/squash_generator_test.go`             | Squash migration generation                           |
| `cmd/go_migrations_test.go`                             | End-to-end generation command                         |
| `internal/interp/loader_test.go`                        | Starlark-Go loader correctness, isolation per-load    |
| `integration_test.go`                                   | Full parse → generate → load → run pipeline           |

---

## Key Architectural Decisions

### 1. .star Files as Source of Truth, Loaded via Starlark-Go

**Decision:** The migration `.star` files in `migrations/` are the source of truth. The CLI loads them in-process via the [Starlark-Go](https://github.com/google/starlark-go) interpreter (`internal/interp.LoadRegistry`) to reconstruct the current schema state. No external compile step or fork/exec is required at runtime.

**Rationale:** Eliminates the `.schema_snapshot.yaml` file that caused merge conflicts in parallel development. The state is derived deterministically from committed `.star` files on every invocation, so it is reproducible and VCS-friendly. Starlark's deterministic, sandboxed execution model is a better fit for migration files than a full Go interpreter: files have no I/O side effects, cannot import arbitrary packages, and evaluate in a consistent order. This removes the runtime Go-toolchain dependency and all related plumbing.

**Tradeoff:** Starlark is not Go; hand-written logic beyond what the generator emits requires learning Starlark syntax. Generated migrations use only the built-in functions exposed by the loader (`migration`, `create_table`, `add_field`, etc.), which are fully documented and stable.

### 2. Top-Level migration() Call Over init() Registration

**Decision:** Migrations register via a top-level `migration()` call in each `.star` file rather than via an `init()` function.

**Rationale:** Starlark has no `init()` concept. Top-level expressions in a `.star` file are evaluated when the file is loaded, which is semantically equivalent. The `migration()` built-in is provided by the loader and writes directly into a per-load `*migrate.Registry` — no global state, no shimming, no ordering surprises. File-system scanning is still used to discover `.star` files, but naming conventions enforced by the generator ensure consistent ordering.

### 3. Typed Operations Over Raw SQL

**Decision:** Migrations express changes as typed `Operation` structs rather than raw SQL strings.

**Rationale:** Typed operations allow `SchemaState` reconstruction by replaying `Mutate()` calls — no database connection required to determine current schema. Operations are also provider-agnostic: the same migration file works against any supported database.

### 4. Kahn's Algorithm with Alphabetical Tie-Breaking

**Decision:** Topological sort uses Kahn's algorithm; nodes at the same level are sorted alphabetically.

**Rationale:** Deterministic ordering is essential: two developers running `migrate up` on the same graph must apply migrations in the same order. Alphabetical tie-breaking is predictable and requires no additional metadata.

### 5. Database Provider Abstraction

**Decision:** All DDL generation is delegated to a `Provider` interface with 12 implementations.

**Rationale:** Isolates database-specific SQL from operation logic. Adding a new database requires implementing one interface, not modifying operation types. Enables mock providers in tests.

### 6. Starlark as Schema Definition Language

**Decision:** Schema is declared in `schema.star`, not inferred from Go structs or a live database.

**Rationale:** Starlark is human-readable, version-control friendly, deterministic, and supports modular composition. It is the single authoritative source of intent; the migration files are derived from it, not the reverse. Using the same language for both schema definitions and migrations reduces the number of file formats developers need to understand.

---

## Module Structure

```
github.com/ocomsoft/morphic
│
├── main.go                        Entry point for the morphic CLI
├── cmd/                           One file per CLI command
│   ├── root.go
│   ├── go_migrations.go           morphic generate (primary — Starlark workflow)
│   ├── go_init.go                 morphic init
│   ├── sql_migrations.go          morphic sql-migrations (legacy)
│   ├── init_sql.go                morphic init --sql (legacy)
│   ├── goose.go                   morphic goose (legacy)
│   ├── db2schema.go
│   ├── struct2schema.go
│   ├── schema2diagram.go
│   ├── schema_to_sql.go
│   └── find_includes.go
│
├── migrate/                       Runtime library used by generated migration files
│   ├── types.go                   Migration, Field, ForeignKey, ManyToMany, Index
│   ├── operations.go              Operation interface + 10 concrete types
│   ├── registry.go                Registry + global Register() + GlobalRegistry()
│   ├── graph.go                   Graph (DAG), BuildGraph, Linearize, ReconstructState
│   ├── state.go                   SchemaState, TableState, mutation methods
│   ├── runner.go                  Runner: Up, Down, Status, ShowSQL
│   ├── recorder.go                MigrationRecorder (morphic_history table)
│   ├── app.go                     App (Cobra CLI invoked in-process by `morphic migrate` or by an optional standalone binary)
│   ├── config.go                  Config for App (DSN, database type)
│   ├── provider_bridge.go         Wires providers.Provider into Runner
│   └── dag_ascii.go               ASCII DAG renderer
│
├── internal/
│   ├── codegen/                   Migration file code generation
│   │   ├── starlark_generator.go  StarlarkGenerator: migration .star files (primary)
│   │   ├── go_generator.go        GoGenerator: migration .go files (legacy format)
│   │   ├── merge_generator.go     MergeGenerator: merge migrations
│   │   └── squash_generator.go    SquashGenerator: squash migrations
│   ├── starlark_loader/           Starlark-Go built-ins and schema loader
│   │   ├── loader.go              Exposes migration(), create_table(), field(), etc.
│   │   └── loader_test.go
│   ├── yaml/                      Schema processing and diff engine
│   │   ├── types.go               Re-exports internal/types
│   │   ├── parser.go              Schema → Schema struct
│   │   ├── diff.go                Schema → SchemaDiff
│   │   ├── state.go               StateManager (.schema_snapshot.yaml, legacy)
│   │   ├── merger.go              Multi-source schema merge
│   │   ├── include_processor.go   Include directive resolution
│   │   ├── module_resolver.go     Go module root discovery
│   │   ├── migration_generator.go Legacy SQL generation
│   │   ├── header.go              Chain metadata (legacy SQL workflow)
│   │   └── chain.go               Chain traversal and fork detection (legacy)
│   ├── config/                    Viper-based config loading
│   ├── types/                     Canonical schema types (Schema, Table, Field, Index)
│   ├── providers/                 12 database provider implementations
│   ├── scanner/                   Schema file discovery
│   ├── struct2schema/             Go AST → schema
│   ├── diff/                      Diff utilities
│   ├── merger/                    Schema merging utilities
│   ├── parser/                    Schema parsing utilities
│   ├── analyzer/                  Schema semantic validation
│   └── writer/                    File writing utilities
│
├── internal/interp/               Starlark-Go loader: evaluates migration .star files
│   ├── loader.go                  LoadRegistry(dir) → *migrate.Registry
│   └── loader_test.go
│
└── migrations/                    Generated per-project (not committed to this repo)
    ├── 0001_initial.star          Generated migration file (evaluated by Starlark-Go)
    ├── 0002_add_users.star        Generated migration file
    └── ...
```
