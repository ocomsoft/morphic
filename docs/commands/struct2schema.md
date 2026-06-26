# struct-to-schema Command

Convert Go struct definitions to morphic-compatible Starlark schema files automatically.

## Overview

The `struct-to-schema` command analyzes Go source code to extract struct definitions and generates comprehensive Starlark schema files that can be used with morphic. This powerful feature enables automatic database schema generation from existing Go codebases.

## Usage

```bash
morphic struct-to-schema [flags]
```

### Flags

- `--input string`: Input directory to scan for Go files (default: ".")
- `--output string`: Output Starlark schema file path (default: "schema/schema.star")  
- `--config string`: Configuration file path for custom type mappings
- `--database string`: Target database type (postgresql, mysql, sqlite, sqlserver) (default: "postgresql")
- `--dry-run`: Preview changes without writing files
- `--verbose`: Show detailed processing information

## Examples

### Basic Usage

Scan the current directory and generate a schema file:
```bash
morphic struct-to-schema
```

### Scan Specific Directory

Scan a models directory and output to a custom location:
```bash
morphic struct-to-schema --input ./models --output schema/generated.star
```

### Preview Changes

See what would be generated without creating files:
```bash
morphic struct-to-schema --dry-run --verbose
```

### Use Custom Configuration

Apply custom type mappings and naming conventions:
```bash
morphic struct-to-schema --config mappings.yaml --database postgresql
```

## Features

### Go Language Support

- **Struct Parsing**: Extracts all exported structs from Go source files
- **Field Analysis**: Analyzes field types, pointers, slices, and embedded structs
- **Package Resolution**: Handles cross-package type references
- **AST-Based**: Uses Go's native AST parser for accurate analysis

### Struct Tag Processing

Supports multiple tag formats with priority order: `db` > `sql` > `gorm` > `bun`

```go
type User struct {
    ID        uint      `db:"id" gorm:"primaryKey"`
    Name      string    `db:"name" gorm:"not null"`
    Email     *string   `db:"email"`
    CreatedAt time.Time `db:"created_at" gorm:"autoCreateTime"`
    Posts     []Post    `gorm:"many2many:user_posts"`
}
```

### Type Mapping

Automatic mapping from Go types to SQL types:

| Go Type | SQL Type | Notes |
|---------|----------|-------|
| `string` | `varchar(255)` | Configurable length |
| `int`, `int32` | `integer` | |
| `int64` | `bigint` | |
| `float32`, `float64` | `float` | |
| `bool` | `boolean` | |
| `time.Time` | `timestamp` | |
| `*T` | nullable `T` | Pointer types are nullable |
| `[]T` | `many_to_many` | Slice types create M2M relationships |
| `sql.NullString` | `varchar(255)` | Nullable by default |

### Relationship Detection

- **Foreign Keys**: Automatically detected from struct type references
- **Many-to-Many**: Generated from slice fields with junction tables
- **Constraints**: Configurable `ON DELETE` and `ON UPDATE` actions
- **Junction Tables**: Auto-generated with proper naming conventions

### Configuration System

Create a `mappings.yaml` file for customization:

```yaml
type_mappings:
  CustomUserID: integer
  MoneyAmount: decimal
  
custom_defaults:
  postgresql:
    now: CURRENT_TIMESTAMP
    new_uuid: gen_random_uuid()
    
table_naming:
  convert_case: snake_case
  prefix: ""
  suffix: ""
```

## Input/Output Examples

### Go Structs Input

```go
package models

import "time"

type User struct {
    ID        uint      `db:"id" gorm:"primaryKey"`
    Name      string    `db:"name" gorm:"not null"`
    Email     *string   `db:"email"`
    CreatedAt time.Time `db:"created_at" gorm:"autoCreateTime"`
    Posts     []Post    `gorm:"many2many:user_posts"`
}

type Post struct {
    ID       uint   `db:"id" gorm:"primaryKey"`
    Title    string `db:"title"`
    AuthorID uint   `db:"author_id"`
    Author   User   `gorm:"foreignKey:AuthorID"`
}
```

### Schema Output

```python
database("generated_schema", "1.0.0")

defaults("postgresql", {
    "blank": "''",
    "now": "CURRENT_TIMESTAMP",
    "new_uuid": "gen_random_uuid()",
})

table("user",
    fields = [
        serial("id", primary_key=True),
        varchar("name", 255),
        varchar("email", 255, nullable=True),
        timestamp("created_at", auto_create=True),
    ],
)

table("post",
    fields = [
        serial("id", primary_key=True),
        varchar("title", 255),
        foreign_key("author_id", fk("user", on_delete="RESTRICT")),
    ],
)

table("user_post",
    fields = [
        foreign_key("user_id", fk("user", on_delete="CASCADE")),
        foreign_key("post_id", fk("post", on_delete="CASCADE")),
    ],
    indexes = [
        index("user_post_unique", ["user_id", "post_id"], unique=True),
    ],
)
```

## Advanced Features

### Schema Merging

When an output file already exists, struct-to-schema will:
1. Create a timestamped backup
2. Merge new tables with existing schema
3. Preserve manual modifications
4. Add new tables and fields
5. Update table structures when needed

### Smart Directory Scanning

Automatically excludes common directories:
- `.git`, `.svn`, `.hg`
- `vendor`, `node_modules`
- `.vscode`, `.idea`
- `tmp`, `temp`, `bin`, `build`

### Error Handling

- **Graceful Failures**: Continues processing if individual files fail
- **Validation**: Ensures generated schema passes morphic validation
- **Detailed Logging**: Verbose mode shows file-by-file progress
- **Recovery**: Creates backups before any destructive operations

## Integration with Morphic

Generated schema files are fully compatible with the morphic workflow:

```bash
# Generate schema from Go structs
morphic struct-to-schema --input ./models

# Generate migration from schema
morphic generate

# Apply migration
morphic goose postgres "connection-string" up
```

## Best Practices

### Struct Design

- Use exported fields for database columns
- Apply appropriate struct tags for constraints
- Use pointers for nullable fields
- Use slices for many-to-many relationships

### Tag Usage

```go
type Model struct {
    ID          uint      `db:"id" gorm:"primaryKey"`
    Name        string    `db:"name" gorm:"not null;size:100"`
    Description *string   `db:"description"`
    CreatedAt   time.Time `db:"created_at" gorm:"autoCreateTime"`
    UpdatedAt   time.Time `db:"updated_at" gorm:"autoUpdateTime"`
    
    // Foreign Key
    CategoryID  uint     `db:"category_id"`
    Category    Category `gorm:"foreignKey:CategoryID;constraint:OnDelete:CASCADE"`
    
    // Many-to-Many
    Tags        []Tag    `gorm:"many2many:model_tags"`
    
    // Ignored field
    Internal    string   `db:"-"`
}
```

### Configuration Tips

- Use custom type mappings for domain-specific types
- Configure table naming conventions consistently
- Set appropriate database defaults for your target database
- Test with `--dry-run` before generating actual files

## Troubleshooting

### Common Issues

**No structs found**: Ensure Go files are in the input directory and structs are exported (capitalized names).

**Type mapping errors**: Check that custom types are properly defined or add them to the configuration file.

**Relationship detection issues**: Verify struct tags are properly formatted and referenced types exist.

**Permission errors**: Ensure write permissions for the output directory.

### Debugging

Use `--verbose` flag for detailed information:
```bash
morphic struct-to-schema --verbose --dry-run
```

This will show:
- Files being scanned  
- Structs discovered
- Relationships detected
- Type mappings applied
- Generated schema preview

## Performance

The struct-to-schema command is optimized for large codebases:
- Efficient AST parsing
- Parallel file processing where possible
- Smart caching of type information
- Minimal memory footprint

Typical performance on a moderately-sized codebase:
- ~100 Go files: < 1 second
- ~1000 Go files: < 5 seconds  
- ~10,000 Go files: < 30 seconds