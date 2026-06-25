# YAML Schema Format Guide

> **Note:** The primary schema format for morphic is now **Starlark** (`schema.star`). YAML (`schema.yaml`) is still supported as a legacy format. For new projects, use `schema.star` — see the [Starlark Migration Format](starlark-migrations.md) and README for examples. Use `morphic yaml2dsl` to convert an existing YAML schema to Starlark.

This guide covers the YAML schema format used by morphic for defining database schemas.

## Overview

The YAML schema format provides a database-agnostic way to define tables, fields, relationships, and constraints that get automatically converted to database-specific SQL.

## Basic Structure

Every schema file follows this structure:

```yaml
database:
  name: string          # Database/application name
  version: string       # Schema version

include:                # Optional: Import schemas from Go modules
  - module: string      # Go module path
    path: string        # Path to schema.yaml within module

defaults:
  database_type:        # Default values for each database
    key: value

tables:
  - name: string        # Table definitions
    fields: []          # Field definitions
```

## Database Section

Defines metadata about your database schema. This is for reference only and does not affect the generated SQL or migrations in anyway.

```yaml
database:
  name: myapp           # Used for documentation and tracking
  version: 1.0.0        # Semantic versioning recommended
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Database or application name |
| `version` | string | Yes | Schema version identifier |

## Include Section

The include section allows you to import schema definitions from external Go modules. This enables schema modularization and reuse across projects.

```yaml
include:
  - module: github.com/example/auth-schemas
    path: schemas/auth.yaml
  - module: github.com/example/user-schemas  
    path: users/schema.yaml
```

### Include Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `module` | string | Yes | Go module path (workspace or go.mod) |
| `path` | string | Yes | Path to schema.yaml within the module |

### How Includes Work

1. **Module Resolution**: Modules are resolved using Go's module system:
   - First checks workspace modules (go.work)
   - Falls back to go.mod dependencies and module cache

2. **Recursive Processing**: Included schemas can have their own includes, creating a dependency tree

3. **Conflict Resolution**: When schemas define the same table or field:
   - **Main schema wins**: Your primary schema takes precedence
   - **Field merging**: Fields from included schemas are added if they don't conflict
   - **Defaults merging**: Main schema defaults override included defaults

4. **Circular Dependency Prevention**: Automatic detection and skipping of circular includes

### Example: Modular Schema Structure

**Main schema.yaml:**
```yaml
database:
  name: myapp
  version: 1.0.0

include:
  - module: github.com/company/auth-module
    path: schemas/auth.yaml
  - module: github.com/company/audit-module
    path: schemas/audit.yaml

defaults:
  postgresql:
    now: CURRENT_TIMESTAMP
    new_uuid: gen_random_uuid()

tables:
  - name: products
    fields:
      - name: id
        type: uuid
        primary_key: true
        default: new_uuid
      - name: name
        type: varchar
        length: 255
        nullable: false
      - name: user_id  # References users table from auth module
        type: foreign_key
        foreign_key:
          table: users
          on_delete: CASCADE
```

**github.com/company/auth-module/schemas/auth.yaml:**
```yaml
database:
  name: auth
  version: 1.0.0

tables:
  - name: users
    fields:
      - name: id
        type: uuid
        primary_key: true
        default: new_uuid
      - name: email
        type: varchar
        length: 255
        nullable: false
      - name: created_at
        type: timestamp
        default: now
        auto_create: true
```

### Benefits of Using Includes

- **Modularity**: Share common schemas across projects
- **Maintainability**: Update shared schemas in one place
- **Consistency**: Ensure consistent field definitions across services
- **Versioning**: Use Go module versioning for schema evolution

### Best Practices for Includes

1. **Organize by Domain**: Group related tables in dedicated modules
   ```
   github.com/company/auth-schemas    # Authentication tables
   github.com/company/audit-schemas   # Audit logging tables
   github.com/company/common-schemas  # Shared lookup tables
   ```

2. **Version Appropriately**: Use semantic versioning for schema modules
   ```go
   // go.mod
   require github.com/company/auth-schemas v1.2.0
   ```

3. **Document Dependencies**: Clearly document which external schemas you depend on

4. **Test Include Resolution**: Ensure all team members can resolve included modules

## Type Mappings Section

The `type_mappings` section lets you override the SQL type that morphic generates for a
given schema field type on a per-database basis. This is useful when the built-in type mapping
is not suitable for your target database.

```yaml
type_mappings:
  postgresql:
    float: "DOUBLE PRECISION"   # override float → DOUBLE PRECISION instead of REAL
    text: "CITEXT"              # use case-insensitive text extension
  sqlserver:
    text: "NVARCHAR(MAX)"       # use unicode text for SQL Server
  mysql:
    uuid: "CHAR(36)"            # explicit char length for UUIDs
```

### Supported Providers

Type mappings can be defined for any supported database:

| Key | Database |
|-----|----------|
| `postgresql` | PostgreSQL |
| `mysql` | MySQL |
| `sqlserver` | SQL Server |
| `sqlite` | SQLite |
| `redshift` | Amazon Redshift |
| `clickhouse` | ClickHouse |
| `tidb` | TiDB |
| `vertica` | Vertica |
| `ydb` | YDB |
| `turso` | Turso |
| `starrocks` | StarRocks |
| `auroradsql` | Aurora DSQL |

### Parameterised Types

For types that take parameters (length, precision, scale), use Go template syntax:

```yaml
type_mappings:
  postgresql:
    decimal: "NUMERIC({{.Precision}},{{.Scale}})"
    varchar: "CHARACTER VARYING({{.Length}})"
```

The available template variables are: `.Length`, `.Precision`, `.Scale`.

### DAG Integration

When `type_mappings` change between runs, morphic automatically generates a
`SetTypeMappings` operation in the migration file. This operation has no SQL effect — it
records the type mapping configuration in the migration DAG so that:

- Subsequent migrations use the correct type overrides
- `db-diff` compares the live database against the correct expected types
- The migration history accurately reflects when type mappings changed

Example generated migration operation:

```go
&m.SetTypeMappings{
    TypeMappings: map[string]string{
        "float": "DOUBLE PRECISION",
    },
},
```

### Built-in Type Mappings

The following table shows the default SQL types used when no `type_mappings` override is set.
Use `type_mappings` to override any of these:

| Schema Type | PostgreSQL | MySQL | SQLite | SQL Server |
|-------------|------------|-------|--------|------------|
| `float` | `REAL` | `FLOAT` | `REAL` | `FLOAT` |
| `text` | `TEXT` | `TEXT` | `TEXT` | `NVARCHAR(MAX)` |
| `boolean` | `BOOLEAN` | `TINYINT(1)` | `INTEGER` | `BIT` |
| `uuid` | `UUID` | `CHAR(36)` | `TEXT` | `UNIQUEIDENTIFIER` |
| `jsonb` | `JSONB` | `JSON` | `TEXT` | `NVARCHAR(MAX)` |

## Defaults Section

Defines database-specific default values that can be referenced in field definitions:

```yaml
defaults:
  postgresql:
    blank: ''
    now: CURRENT_TIMESTAMP
    new_uuid: gen_random_uuid()
    today: CURRENT_DATE
    zero: "0"
    true: "true"
    false: "false"
    null: "null"
  
  mysql:
    blank: ''
    now: CURRENT_TIMESTAMP
    new_uuid: (UUID())
    today: (CURDATE())
    zero: "0"
    true: "1"
    false: "0"
    null: "null"
```

### Built-in Defaults

Each database type includes these standard defaults:

| Default | PostgreSQL | MySQL | SQLite | SQL Server |
|---------|------------|-------|--------|------------|
| `blank` | `''` | `''` | `''` | `''` |
| `now` | `CURRENT_TIMESTAMP` | `CURRENT_TIMESTAMP` | `CURRENT_TIMESTAMP` | `GETDATE()` |
| `today` | `CURRENT_DATE` | `(CURDATE())` | `CURRENT_DATE` | `CAST(GETDATE() AS DATE)` |
| `new_uuid` | `gen_random_uuid()` | `(UUID())` | `''` | `NEWID()` |
| `zero` | `"0"` | `"0"` | `"0"` | `"0"` |
| `true` | `"true"` | `"1"` | `"1"` | `"1"` |
| `false` | `"false"` | `"0"` | `"0"` | `"0"` |

## Tables Section

Defines the database tables and their structure:

```yaml
tables:
  - name: users
    fields:
      - name: id
        type: uuid
        primary_key: true
        default: new_uuid
      - name: email
        type: varchar
        length: 255
        nullable: false
      - name: created_at
        type: timestamp
        default: now
        auto_create: true
```

### Table Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Table name (snake_case recommended) |
| `fields` | array | Yes | List of field definitions |

## Field Definitions

Fields define the columns in your database tables:

```yaml
- name: field_name
  type: field_type
  # ... additional properties
```

### Basic Field Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `name` | string | Required | Field name (snake_case recommended) |
| `type` | string | Required | Field data type |
| `nullable` | boolean | `true` | Whether field accepts NULL values |
| `primary_key` | boolean | `false` | Whether field is primary key |
| `default` | string | none | Default value or reference |

### Field Type Properties

| Property | Type | Applies To | Description |
|----------|------|------------|-------------|
| `length` | integer | varchar, text | Maximum character length |
| `precision` | integer | decimal | Total number of digits |
| `scale` | integer | decimal | Number of decimal places |
| `auto_create` | boolean | timestamp | Set to NOW() on INSERT |
| `auto_update` | boolean | timestamp | Set to NOW() on UPDATE |

## Data Types

### Basic Types

| Type | PostgreSQL | MySQL | SQLite | SQL Server | Description |
|------|------------|-------|--------|------------|-------------|
| `varchar` | VARCHAR(n) | VARCHAR(n) | TEXT | VARCHAR(n) | Variable-length string |
| `text` | TEXT | TEXT | TEXT | NVARCHAR(MAX) | Large text field |
| `integer` | INTEGER | INT | INTEGER | INT | 32-bit integer |
| `bigint` | BIGINT | BIGINT | INTEGER | BIGINT | 64-bit integer |
| `serial` | SERIAL | AUTO_INCREMENT | INTEGER | IDENTITY | Auto-incrementing integer |
| `float` | REAL | FLOAT | REAL | FLOAT | Floating point number |
| `decimal` | DECIMAL(p,s) | DECIMAL(p,s) | NUMERIC | DECIMAL(p,s) | Fixed-point decimal |
| `boolean` | BOOLEAN | TINYINT(1) | INTEGER | BIT | Boolean true/false |
| `timestamp` | TIMESTAMP | TIMESTAMP | DATETIME | DATETIME2 | Date and time |
| `date` | DATE | DATE | DATE | DATE | Date only |
| `time` | TIME | TIME | TIME | TIME | Time only |
| `uuid` | UUID | CHAR(36) | TEXT | UNIQUEIDENTIFIER | UUID/GUID |
| `jsonb` | JSONB | JSON | TEXT | NVARCHAR(MAX) | JSON data |

### String Types

```yaml
# Fixed-length string (specify length)
- name: username
  type: varchar
  length: 255
  nullable: false

# Large text field
- name: description
  type: text
  length: 5000        # Optional length limit
  nullable: true
  default: blank
```

### Numeric Types

```yaml
# Integer types
- name: age
  type: integer
  nullable: true

- name: user_count
  type: bigint
  default: zero

# Auto-incrementing primary key
- name: id
  type: serial
  primary_key: true

# Decimal with precision and scale
- name: price
  type: decimal
  precision: 10       # Total digits
  scale: 2           # Decimal places
  nullable: false
```

### Date and Time Types

```yaml
# Timestamp with auto-creation
- name: created_at
  type: timestamp
  default: now
  auto_create: true   # Set on INSERT

# Timestamp with auto-update
- name: updated_at
  type: timestamp
  nullable: true
  auto_update: true   # Set on UPDATE

# Date only
- name: birth_date
  type: date
  nullable: true
  default: today

# Time only
- name: daily_reminder
  type: time
  default: "09:00:00"
```

### UUID and JSON Types

```yaml
# UUID primary key
- name: id
  type: uuid
  primary_key: true
  default: new_uuid

# JSON data
- name: metadata
  type: jsonb
  nullable: true
  default: object     # Defaults to '{}'
```

## Relationships

### Foreign Keys

Define relationships between tables:

```yaml
- name: user_id
  type: foreign_key
  nullable: false
  foreign_key:
    table: users
    on_delete: CASCADE    # CASCADE, RESTRICT, SET_NULL, PROTECT
```

#### Foreign Key Options

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `table` | string | Yes | Referenced table name |
| `on_delete` | string | No | Deletion behavior |

**On Delete Options:**
- `CASCADE` - Delete referencing rows
- `RESTRICT` - Prevent deletion if references exist
- `SET_NULL` - Set foreign key to NULL
- `PROTECT` - Same as RESTRICT (default)

### Many-to-Many Relationships

Many-to-many relationships are implemented using explicit junction tables. Create a separate table to represent the relationship:

```yaml
# Main tables
- name: products
  fields:
    - name: id
      type: serial
      primary_key: true
    - name: name
      type: varchar
      length: 255

- name: categories  
  fields:
    - name: id
      type: serial
      primary_key: true
    - name: name
      type: varchar
      length: 255

# Junction table for many-to-many relationship
- name: product_categories
  fields:
    - name: id
      type: serial
      primary_key: true
    - name: product_id
      type: foreign_key
      nullable: false
      foreign_key:
        table: products
        on_delete: CASCADE
    - name: category_id
      type: foreign_key
      nullable: false
      foreign_key:
        table: categories
        on_delete: CASCADE
  indexes:
    - name: idx_product_categories_unique
      fields: [product_id, category_id]
      unique: true
```

This approach provides explicit control over the junction table structure and allows for additional fields if needed.

## Indexes

Define database indexes at the table level to improve query performance and enforce uniqueness constraints:

```yaml
tables:
  - name: users
    fields:
      - name: id
        type: uuid
        primary_key: true
      - name: email
        type: varchar
        length: 255
        nullable: false
      - name: username
        type: varchar
        length: 100
        nullable: false
    indexes:
      - name: idx_users_email
        fields: [email]
        unique: true
      - name: idx_users_username
        fields: [username]
        unique: true
```

### Index Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Index name (must be unique within database) |
| `fields` | array | Yes | List of field names to include in index |
| `unique` | boolean | No | Whether to create a unique index (default: false) |

### Multi-Column Indexes

Create composite indexes for queries that filter on multiple columns:

```yaml
indexes:
  - name: idx_users_name_email
    fields: [last_name, first_name, email]
    unique: false
```

### Unique Constraints

Use unique indexes to enforce business rules:

```yaml
tables:
  - name: accounts
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: account_number
        type: varchar
        length: 20
        nullable: false
      - name: branch_code
        type: varchar
        length: 10
        nullable: false
    indexes:
      - name: idx_accounts_account_number
        fields: [account_number]
        unique: true
      - name: idx_accounts_branch_account
        fields: [branch_code, account_number]
        unique: true  # Unique per branch
```

### Index Naming Conventions

- Use `idx_` prefix for regular indexes
- Use `uniq_` prefix for unique indexes (optional)
- Include table name and field names: `idx_tablename_field1_field2`
- Keep names under database limits (usually 63 characters)

### Database-Specific Behavior

**PostgreSQL:**
- Supports partial indexes (not yet implemented in schema)
- B-tree indexes by default
- Unique indexes can be used as constraints

**MySQL:**
- Automatically creates indexes for foreign keys
- Index names must be unique within table
- Maximum index length varies by storage engine

**SQLite:**
- Supports unique indexes
- Limited index options compared to other databases

**SQL Server:**
- Clustered and non-clustered indexes (uses non-clustered)
- Unique indexes enforce uniqueness

## Default Values

### Using Default References

```yaml
- name: created_at
  type: timestamp
  default: now        # References defaults.database_type.now

- name: email
  type: varchar
  length: 255
  default: blank      # References defaults.database_type.blank

- name: is_active
  type: boolean
  default: true       # References defaults.database_type.true
```

### Literal Default Values

```yaml
- name: status
  type: varchar
  length: 50
  default: "pending"  # Literal string value

- name: priority
  type: integer
  default: 1          # Literal numeric value

- name: config
  type: jsonb
  default: '{"enabled": true}'  # Literal JSON
```

## Complete Examples

### User Management Schema

```yaml
database:
  name: user_management
  version: 1.0.0

include:
  - module: github.com/company/audit-schemas
    path: schemas/audit.yaml

defaults:
  postgresql:
    blank: ''
    now: CURRENT_TIMESTAMP
    new_uuid: gen_random_uuid()
    true: "true"
    false: "false"

tables:
  - name: users
    fields:
      - name: id
        type: uuid
        primary_key: true
        default: new_uuid
      - name: email
        type: varchar
        length: 255
        nullable: false
      - name: username
        type: varchar
        length: 100
        nullable: false
      - name: password_hash
        type: varchar
        length: 255
        nullable: false
      - name: is_active
        type: boolean
        default: true
      - name: created_at
        type: timestamp
        default: now
        auto_create: true
      - name: updated_at
        type: timestamp
        nullable: true
        auto_update: true

  - name: user_profiles
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: user_id
        type: foreign_key
        nullable: false
        foreign_key:
          table: users
          on_delete: CASCADE
      - name: first_name
        type: varchar
        length: 100
        nullable: true
      - name: last_name
        type: varchar
        length: 100
        nullable: true
      - name: bio
        type: text
        length: 1000
        nullable: true
        default: blank
      - name: avatar_url
        type: varchar
        length: 500
        nullable: true
```

### E-commerce Schema

```yaml
database:
  name: ecommerce
  version: 2.1.0

defaults:
  postgresql:
    blank: ''
    now: CURRENT_TIMESTAMP
    new_uuid: gen_random_uuid()
    zero: "0"
    object: '{}'

tables:
  - name: products
    fields:
      - name: id
        type: uuid
        primary_key: true
        default: new_uuid
      - name: name
        type: varchar
        length: 255
        nullable: false
      - name: description
        type: text
        nullable: true
      - name: price
        type: decimal
        precision: 10
        scale: 2
        nullable: false
      - name: inventory_count
        type: integer
        default: zero
      - name: metadata
        type: jsonb
        nullable: true
        default: object
      - name: is_active
        type: boolean
        default: true
      - name: created_at
        type: timestamp
        default: now
        auto_create: true

  - name: categories
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: name
        type: varchar
        length: 100
        nullable: false
      - name: slug
        type: varchar
        length: 100
        nullable: false
      - name: parent_id
        type: foreign_key
        nullable: true
        foreign_key:
          table: categories
          on_delete: SET_NULL

  - name: product_categories
    fields:
      - name: id
        type: serial
        primary_key: true
      - name: product_id
        type: foreign_key
        nullable: false
        foreign_key:
          table: products
          on_delete: CASCADE
      - name: category_id
        type: foreign_key
        nullable: false
        foreign_key:
          table: categories
          on_delete: CASCADE
```

## Best Practices

### Naming Conventions

**Tables:**
- Use `snake_case` naming
- Use plural nouns (e.g., `users`, `products`)
- Be descriptive but concise

**Fields:**
- Use `snake_case` naming
- Use descriptive names
- Foreign keys: `{table}_id` (e.g., `user_id`)
- Timestamps: `created_at`, `updated_at`
- Booleans: `is_active`, `has_permission`

### Schema Organization

**File Structure:**
```
schema/
├── schema.yaml           # Main schema file with includes
├── core/
│   ├── users.yaml       # User-related tables
│   └── auth.yaml        # Authentication tables
└── modules/
    ├── products.yaml    # Product catalog
    └── orders.yaml      # Order management
```

**Schema Splitting:**
- Keep related tables together
- Separate core functionality from modules
- Use consistent naming across files
- Use includes for external module schemas
- Prefer local files for project-specific schemas

### Data Types

**Choose appropriate types:**
```yaml
# Good
- name: id
  type: uuid              # For distributed systems
  
- name: email
  type: varchar
  length: 255             # Standard email length

- name: price
  type: decimal
  precision: 10
  scale: 2               # For monetary values

# Avoid
- name: id
  type: varchar          # Don't use strings for IDs
  
- name: price
  type: float            # Don't use float for money
```

### Relationships

**Foreign Key Best Practices:**
```yaml
# Explicit relationship
- name: user_id
  type: foreign_key
  nullable: false        # Require the relationship
  foreign_key:
    table: users
    on_delete: CASCADE   # Be explicit about cascading

# Self-referencing relationship
- name: parent_id
  type: foreign_key
  nullable: true         # Allow root items
  foreign_key:
    table: categories    # Reference same table
    on_delete: SET_NULL  # Preserve children
```

### Default Values

**Use semantic defaults:**
```yaml
# Good
- name: created_at
  type: timestamp
  default: now           # Semantic reference

- name: is_active
  type: boolean
  default: true          # Meaningful default

# Avoid
- name: created_at
  type: timestamp
  default: CURRENT_TIMESTAMP  # Database-specific syntax
```

## Migration Considerations

### Destructive Changes

These operations require review and may prompt for confirmation:

- **Removing tables** (`table_removed`)
- **Removing fields** (`field_removed`)
- **Renaming tables** (`table_renamed`)
- **Renaming fields** (`field_renamed`)
- **Modifying field types** (`field_modified`)

### Safe Changes

These operations are generally safe and won't prompt:

- Adding tables
- Adding fields
- Adding indexes
- Modifying nullable to true
- Increasing varchar length

### Schema Evolution

**Version your schemas:**
```yaml
database:
  name: myapp
  version: 2.1.0         # Update on breaking changes
```

**Backward compatibility:**
- Add new fields as nullable
- Use default values for new required fields
- Deprecate before removing

## Validation and Debugging

### Schema Validation

```bash
# Validate schema syntax
morphic generate --dry-run --verbose

# Check for issues
morphic schema-to-sql --verbose
```

### Common Issues

**YAML Syntax:**
```yaml
# Wrong indentation
tables:
- name: users           # Missing space after -
  fields:

# Correct indentation  
tables:
  - name: users         # Proper spacing
    fields:
```

**Data Type Issues:**
```yaml
# Missing required properties
- name: email
  type: varchar         # Missing length for varchar

# Correct
- name: email
  type: varchar
  length: 255
```

**Relationship Issues:**
```yaml
# Missing foreign_key definition
- name: user_id
  type: foreign_key     # Missing foreign_key section

# Correct
- name: user_id
  type: foreign_key
  foreign_key:
    table: users
```

## Advanced Features

### Custom Default Values

```yaml
defaults:
  postgresql:
    blank: ''
    now: CURRENT_TIMESTAMP
    company_id: '12345'    # Custom default
    api_version: 'v1'      # Custom string default
```

### Complex JSON Defaults

```yaml
- name: settings
  type: jsonb
  default: '{"theme": "light", "notifications": true}'

- name: permissions
  type: jsonb
  default: object        # Uses defaults.database_type.object
```

### Self-Referencing Tables

```yaml
- name: categories
  fields:
    - name: id
      type: serial
      primary_key: true
    - name: name
      type: varchar
      length: 100
    - name: parent_id
      type: foreign_key
      nullable: true
      foreign_key:
        table: categories  # Self-reference
        on_delete: SET_NULL
```

For implementation examples, see the `/example` directory.
For command usage, see the [Commands Documentation](commands/).
For configuration options, see the [Configuration Guide](configuration.md).
