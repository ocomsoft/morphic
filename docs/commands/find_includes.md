# find-includes

The `find-includes` command automatically discovers schema files in Go modules and workspace, then adds them as includes to your main schema.star file.

## Overview

This command helps you manage schema includes by:
- Automatically discovering schema.star files in Go workspace modules and dependencies
- Adding discovered schemas to your main schema's include section
- Preserving existing includes (only adds new ones)
- Supporting both automatic and interactive selection modes

## Usage

```bash
morphic find-includes [flags]
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--schema` | (auto-detect) | Path to the main schema file to update. If not provided, searches for schema.star files in the current directory |
| `--interactive` | `false` | Review and select which schemas to include |
| `--workspace` | `true` | Include workspace modules in discovery |
| `--verbose` | `false` | Show detailed processing information |

## Schema Auto-Detection

When the `--schema` flag is not provided, the command will:

1. **Recursively search** for all `schema.star` files in the current directory and subdirectories
2. **If one file is found**: Automatically use it as the target schema
3. **If multiple files are found**: Prompt you to select which one to update, showing:
   - File path
   - Database name
   - Number of tables

Example prompt when multiple schemas are found:
```
Multiple schema.star files found:
===================================

1. Path: example/ocom/schema/schema.star
   Database: ocom
   Tables: 2

2. Path: example/schema/schema.star
   Database: app
   Tables: 8

Which schema file would you like to update? [1-2]: 
```

## Discovery Scope

The command searches for schema.star files in:

1. **Go workspace modules** (if go.work exists)
   - Prioritized and marked as "recommended"
   - Controlled by `--workspace` flag
2. **Direct dependencies** in go.mod
   - Only direct dependencies (not transitive/indirect)

## Examples

### Basic Usage (Auto-detect schema)
```bash
# Automatically find and update schema.star in current directory tree
morphic find-includes
```

### Specify Schema File
```bash
# Update a specific schema file
morphic find-includes --schema schema/schema.star
```

### Interactive Mode
```bash
# Review each discovered schema before adding
morphic find-includes --interactive
```

### Verbose Output
```bash
# See detailed discovery process
morphic find-includes --verbose
```

### Exclude Workspace Modules
```bash
# Only discover schemas from go.mod dependencies
morphic find-includes --workspace=false
```

## Interactive Mode

When using `--interactive`, you'll be prompted for each discovered schema:

```
Discovered schemas (workspace modules are recommended):
======================================================

1. Module: github.com/ocomsoft/ocom
   Path: schema/schema.star
   Database: ocom
   Tables: 15
   Type: Workspace module (recommended)

   Include this schema? [Y/n]: 
```

## Output

After successful execution, the command shows:
- Number of includes added
- Path to the updated schema file
- List of added includes with their module paths

Example output:
```
Successfully added 2 include(s) to schema/schema.star

Added includes:
  - github.com/ocomsoft/ocom -> schema/schema.star (workspace)
  - github.com/example/lib -> db/schema.star
```

## Include Format

The command adds includes in the following Starlark format to your schema file:

```yaml
include:
  - module: github.com/ocomsoft/ocom
    path: schema/schema.star
  - module: github.com/example/lib
    path: db/schema.star
```

## Notes

- The command only adds new includes; it never removes or modifies existing ones
- Workspace modules are discovered from `go.work` file
- Module dependencies are discovered from `go.mod` file
- The command preserves the existing structure and formatting of your schema file
- Schema files must be named exactly `schema.star` to be discovered