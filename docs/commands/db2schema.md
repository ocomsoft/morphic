# db-to-schema Command

The `db-to-schema` command extracts database schema information from a PostgreSQL database and generates a Starlark schema file compatible with morphic. This reverse engineering tool allows you to convert existing database structures into morphic schema format.

## Overview

The `db-to-schema` command connects to a PostgreSQL database, reads the INFORMATION_SCHEMA tables, and extracts complete metadata to generate a comprehensive Starlark schema file. It's useful for:

- Converting existing databases to morphic format
- Reverse engineering database structures for documentation
- Migrating from other schema management tools
- Creating schema files from production databases
- Generating starting points for new schema-driven development

## Usage

```bash
morphic db-to-schema [flags]
```

## Command Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--host` | string | `localhost` | Database host |
| `--port` | int | `5432` | Database port |
| `--database` | string | `postgres` | Database name |
| `--username` | string | `postgres` | Database username |
| `--password` | string | | Database password |
| `--sslmode` | string | `disable` | SSL mode (disable, require, verify-ca, verify-full) |
| `--output` | string | `schema.star` | Output Starlark schema file path |
| `--verbose` | bool | `false` | Show detailed processing information |

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `migrations/morphic.config.yaml` | Path to configuration file |

## What It Extracts

### 1. Database Tables
Extracts all user-defined tables from the public schema:

```python
table("users",
    fields = [...],
)
table("products",
    fields = [...],
)
table("orders",
    fields = [...],
)
```

### 2. Field Information
Complete field metadata including:
- **Name**: Column name
- **Data Type**: Converted to morphic schema types
- **Length**: For VARCHAR and TEXT fields
- **Precision/Scale**: For DECIMAL fields  
- **Nullable**: Whether the field accepts NULL values
- **Primary Key**: Primary key constraints
- **Default Values**: Converted to morphic format

```python
fields = [
    uuid("id", primary_key=True, default="new_uuid"),
    varchar("email", 255),
    timestamp("created_at", default="now"),
]
```

### 3. Foreign Key Relationships
Extracts foreign key constraints with ON DELETE actions:

```python
foreign_key("user_id", fk("users", on_delete="CASCADE")),
```

### 4. Indexes
All indexes including unique constraints:

```python
indexes = [
    index("idx_users_email", ["email"], unique=True),
    index("idx_users_created_at", ["created_at"]),
]
```

### 5. Default Values
Intelligent conversion of SQL defaults to schema format:

| SQL Default | Schema Default | Description |
|-------------|--------------|-------------|
| `CURRENT_TIMESTAMP` | `now` | Current timestamp |
| `CURRENT_DATE` | `today` | Current date |
| `gen_random_uuid()` | `new_uuid` | PostgreSQL UUID generation |
| `true` | `true` | Boolean true |
| `false` | `false` | Boolean false |
| `''` | `blank` | Empty string |
| `0` | `zero` | Numeric zero |

## Examples

### Basic Usage

```bash
# Extract schema from local PostgreSQL
morphic db-to-schema --database=myapp --username=myuser --password=mypass

# Output
Database schema successfully extracted to: schema.star

Extracted 5 tables:
  - users
  - products
  - categories
  - orders
  - order_items

You can now use this schema file with other morphic commands.
```

### Verbose Mode

```bash
# Show detailed processing information
morphic db-to-schema --verbose --database=myapp --username=myuser

# Output
Extracting database schema to Starlark
======================================
Database type: postgresql
Output file: schema.star

1. Connecting to database...
Successfully extracted schema with 5 tables
  - users: 8 fields, 2 indexes
  - products: 6 fields, 3 indexes
  - categories: 4 fields, 1 indexes
  - orders: 7 fields, 2 indexes
  - order_items: 5 fields, 2 indexes

2. Converting to Starlark format...

3. Writing Starlark schema file...
Database schema successfully extracted to: schema.star
```

### Custom Output Location

```bash
# Extract to specific file
morphic db-to-schema --output=extracted_schema.star --database=myapp

# Extract to directory
morphic db-to-schema --output=schemas/production.star --database=prod_db
```

### Remote Database Connection

```bash
# Connect to remote database
morphic db-to-schema \
  --host=db.example.com \
  --port=5432 \
  --database=production \
  --username=readonly \
  --password=secretpass \
  --sslmode=require \
  --output=production_schema.star
```

### Using Environment Variables

```bash
# Set connection via environment
export PGHOST=localhost
export PGPORT=5432
export PGDATABASE=myapp
export PGUSER=myuser
export PGPASSWORD=mypass

# Extract schema
morphic db-to-schema --verbose
```

## Database Support

### PostgreSQL (Full Support)
- ✅ All data types supported
- ✅ Primary keys and foreign keys
- ✅ Unique and regular indexes
- ✅ Default values and constraints
- ✅ Nullable field detection
- ✅ SERIAL and auto-increment fields
- ✅ ON DELETE cascade rules

### Other Databases (Planned)
- 🔄 MySQL - Placeholder implementation
- 🔄 SQLite - Placeholder implementation  
- 🔄 SQL Server - Placeholder implementation
- 🔄 Oracle - Planned for future release

## Generated Starlark Structure

The command generates a complete Starlark schema file:

```python
database("extracted_schema", "1.0.0")

defaults("postgresql", {
    "blank": "''",
    "now": "CURRENT_TIMESTAMP",
    "new_uuid": "gen_random_uuid()",
    "today": "CURRENT_DATE",
    "zero": "'0'",
    "true": "'true'",
    "false": "'false'",
    "null": "null",
})

table("users",
    fields = [
        uuid("id", primary_key=True, default="new_uuid"),
        varchar("email", 255),
        varchar("username", 100),
        varchar("password_hash", 255),
        boolean("is_active", nullable=True, default="true"),
        timestamp("created_at", default="now"),
        timestamp("updated_at", nullable=True),
    ],
    indexes = [
        index("idx_users_email", ["email"], unique=True),
        index("idx_users_username", ["username"], unique=True),
    ],
)

table("user_profiles",
    fields = [
        serial("id", primary_key=True),
        foreign_key("user_id", fk("users", on_delete="CASCADE")),
        varchar("first_name", 100, nullable=True),
        varchar("last_name", 100, nullable=True),
        text("bio", nullable=True, default="blank"),
    ],
)
```

## Type Mapping

### PostgreSQL to Schema Type Conversion

| PostgreSQL Type | Schema Type | Notes |
|-----------------|-------------|-------|
| `VARCHAR(n)` | `varchar` | Length preserved |
| `TEXT` | `text` | |
| `INTEGER` | `integer` | |
| `BIGINT` | `bigint` | |
| `SERIAL` | `serial` | Auto-increment |
| `REAL` | `float` | |
| `NUMERIC(p,s)` | `decimal` | Precision/scale preserved |
| `BOOLEAN` | `boolean` | |
| `DATE` | `date` | |
| `TIME` | `time` | |
| `TIMESTAMP` | `timestamp` | |
| `UUID` | `uuid` | |
| `JSONB` | `jsonb` | |
| `JSON` | `jsonb` | Converted to JSONB |

### Serial Field Detection

The command intelligently detects PostgreSQL SERIAL fields:

```sql
-- PostgreSQL table
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255)
);
```

```python
# Generated schema
serial("id", primary_key=True)
```

## Connection Security

### SSL Modes

| SSL Mode | Description | Use Case |
|----------|-------------|----------|
| `disable` | No SSL connection | Local development |
| `require` | SSL required, no verification | Basic security |
| `verify-ca` | SSL with CA verification | Trusted networks |
| `verify-full` | Full SSL verification | Production systems |

### Example with SSL

```bash
# Production connection with full SSL verification
morphic db-to-schema \
  --host=prod-db.company.com \
  --port=5432 \
  --database=production \
  --username=readonly \
  --sslmode=verify-full \
  --verbose
```

## Error Handling

### Connection Errors

```bash
$ morphic db-to-schema --host=nonexistent --database=test
Error: failed to extract database schema: failed to ping database: 
dial tcp: lookup nonexistent: no such host
```

### Authentication Errors

```bash
$ morphic db-to-schema --username=invalid --password=wrong
Error: failed to extract database schema: failed to ping database: 
pq: password authentication failed for user "invalid"
```

### Database Access Errors

```bash
$ morphic db-to-schema --database=forbidden
Error: failed to extract database schema: failed to ping database:
pq: permission denied for database "forbidden"
```

### Permission Errors

```bash
$ morphic db-to-schema --username=limited_user
Error: failed to extract database schema: failed to extract tables: 
failed to query tables: pq: permission denied for schema information_schema
```

## Troubleshooting

### Common Issues and Solutions

1. **Connection Refused**
   ```bash
   # Check if PostgreSQL is running
   sudo systemctl status postgresql
   
   # Check port and host
   netstat -tlnp | grep 5432
   ```

2. **Permission Denied**
   ```bash
   # Grant necessary permissions
   GRANT USAGE ON SCHEMA information_schema TO readonly_user;
   GRANT SELECT ON ALL TABLES IN SCHEMA information_schema TO readonly_user;
   ```

3. **Empty Schema Output**
   ```bash
   # Check if tables exist in public schema
   psql -d mydb -c "\dt"
   
   # Check schema search path
   psql -d mydb -c "SHOW search_path;"
   ```

4. **Type Conversion Issues**
   ```bash
   # Use verbose mode to see detailed type processing
   morphic db-to-schema --verbose --database=mydb
   ```

## Integration with Morphic

### Workflow Example

```bash
# 1. Extract existing database schema
morphic db-to-schema --database=production --output=baseline_schema.star

# 2. Initialize migrations directory with extracted schema
morphic init --schema=baseline_schema.star

# 3. Make schema modifications
vim schema/schema.star

# 4. Generate migration for changes
morphic generate --name="add_new_features"

# 5. Apply migrations
goose -dir migrations postgres $DATABASE_URL up
```

### Schema Evolution Workflow

```bash
# Before making changes - capture current state
morphic db-to-schema --database=staging --output=current_state.star

# Make schema changes in Starlark
vim schema/schema.star

# Compare what will change
diff current_state.star schema/schema.star

# Generate and review migration
morphic generate --dry-run
```

## Performance Considerations

### Large Databases

For databases with many tables:

```bash
# Use verbose mode to monitor progress
time morphic db-to-schema --verbose --database=large_db

# Consider extracting specific schemas (future feature)
# morphic db-to-schema --schema=specific_schema --database=large_db
```

### Network Considerations

```bash
# For remote databases, consider connection timeouts
timeout 300 morphic db-to-schema \
  --host=remote-db.example.com \
  --database=myapp \
  --verbose
```

## Configuration Integration

The command respects the configuration file for database type settings:

```yaml
# migrations/morphic.config.yaml
database:
  type: postgresql              # Used to determine provider
  default_schema: public        # Schema to extract from
  quote_identifiers: true       # How to handle identifiers
```

### Environment Variable Support

```bash
# Database connection can be specified via environment
export MORPHIC_DATABASE_TYPE=postgresql
export DATABASE_URL=postgres://user:pass@host:port/dbname

# Run extraction
morphic db-to-schema --verbose
```

## Use Cases

### 1. Legacy Database Migration

```bash
# Extract legacy database structure
morphic db-to-schema \
  --host=legacy-db.company.com \
  --database=legacy_system \
  --username=readonly \
  --output=legacy_schema.star

# Review and clean up generated schema
vim legacy_schema.star

# Initialize new schema-based project
morphic init --schema=legacy_schema.star
```

### 2. Development Environment Setup

```bash
# Extract production schema
morphic db-to-schema \
  --host=prod-db.company.com \
  --database=production \
  --username=readonly \
  --sslmode=verify-full \
  --output=prod_schema.star

# Use for local development
cp prod_schema.star schema/schema.star
morphic init
```

### 3. Documentation Generation

```bash
# Extract schema for documentation
morphic db-to-schema --database=myapp --verbose > schema_extraction.log
morphic schema-to-sql --database=postgresql > schema_structure.sql

# Generate complete documentation
echo "# Database Schema Documentation" > docs/database.md
echo "## Extraction Log" >> docs/database.md
cat schema_extraction.log >> docs/database.md
echo "## SQL Structure" >> docs/database.md
cat schema_structure.sql >> docs/database.md
```

### 4. Schema Comparison

```bash
# Compare staging vs production
morphic db-to-schema --database=staging --output=staging_schema.star
morphic db-to-schema --database=production --output=prod_schema.star

# Compare schemas
diff staging_schema.star prod_schema.star
```

### 5. CI/CD Integration

```bash
#!/bin/bash
# ci/extract-schema.sh

# Extract current production schema
morphic db-to-schema \
  --host=$PROD_DB_HOST \
  --database=$PROD_DB_NAME \
  --username=$READONLY_USER \
  --password=$READONLY_PASS \
  --output=current_prod_schema.star

# Compare with committed schema
if ! diff current_prod_schema.star schema/schema.star > /dev/null; then
    echo "Schema drift detected between code and production!"
    echo "Differences:"
    diff current_prod_schema.star schema/schema.star
    exit 1
fi
```

## Advanced Features

### Custom Database Names

```bash
# Override extracted database name
morphic db-to-schema --database=myapp --output=temp_schema.star

# Edit the database section
sed -i 's/extracted_schema/myapp_v2/g' temp_schema.star
```

### Selective Table Extraction (Future Feature)

```bash
# Future planned feature
# morphic db-to-schema --tables="users,products,orders" --database=myapp
```

### Schema Filtering (Future Feature)

```bash
# Future planned feature  
# morphic db-to-schema --exclude-tables="audit_*,temp_*" --database=myapp
```

## Migration Path Examples

### From Django to Morphic

```bash
# 1. Extract Django database
morphic db-to-schema --database=django_app --output=django_schema.star

# 2. Convert Django-specific types (manual review needed)
# - Review auto_now and auto_now_add fields
# - Check CharField max_length mappings
# - Verify foreign key on_delete behaviors

# 3. Initialize morphic
morphic init --schema=django_schema.star
```

### From Rails to Morphic  

```bash
# 1. Extract Rails database
morphic db-to-schema --database=rails_app --output=rails_schema.star

# 2. Review Rails conventions
# - Convert created_at/updated_at to auto_create/auto_update
# - Review ActiveRecord foreign key naming
# - Check index naming conventions

# 3. Initialize morphic
morphic init --schema=rails_schema.star
```

### From Liquibase to Morphic

```bash
# 1. Extract current database state
morphic db-to-schema --database=liquibase_db --output=current_schema.star

# 2. Clean up and organize
# - Remove Liquibase system tables from extracted schema
# - Review and consolidate field definitions
# - Standardize naming conventions

# 3. Initialize clean migration history
morphic init --schema=current_schema.star
```

## Limitations and Considerations

### Current Limitations

1. **Schema Scope**: Only extracts from `public` schema
2. **PostgreSQL Only**: Full support limited to PostgreSQL currently  
3. **Basic Types**: Complex types may map to simpler schema equivalents
4. **Views**: Database views are not extracted
5. **Procedures**: Stored procedures and functions not included
6. **Triggers**: Database triggers not extracted
7. **Partitioning**: Partitioned tables treated as regular tables

### Data Type Considerations

Some PostgreSQL types have no direct schema equivalent:
- Arrays → converted to text
- Custom types → converted to text
- Geometric types → converted to text
- Network types → converted to text

### Performance Considerations

- Large databases may take time to process
- Network latency affects remote database extraction
- Complex schemas with many foreign keys require more processing time

## See Also

- [init Command](./init.md) - Initialize new projects with extracted schemas
- [morphic Command](./morphic.md) - Generate migrations from schemas  
- [schema-to-sql Command](./schema_to_sql.md) - View generated SQL from extracted schemas
- [Schema Format Guide](../schema-format.md) - Starlark schema syntax reference
- [Configuration Guide](../configuration.md) - Configuration options