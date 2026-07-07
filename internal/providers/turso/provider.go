/*
MIT License

# Copyright (c) 2025 OcomSoft

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/
package turso

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
)

// Provider implements the Provider interface for Turso
// Turso is a distributed SQLite-compatible database for edge computing
type Provider struct {
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new Turso provider
func New() *Provider {
	return &Provider{}
}

// Placeholder returns the bind-parameter placeholder for the nth argument (1-indexed).
func (p *Provider) Placeholder(_ int) string {
	return "?"
}

// HistoryTableDDL returns the CREATE TABLE IF NOT EXISTS statement for the
// morphic_history migration-tracking table, using this provider's SQL dialect.
func (p *Provider) HistoryTableDDL() string {
	return `CREATE TABLE IF NOT EXISTS morphic_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    applied_at TEXT DEFAULT CURRENT_TIMESTAMP
)`
}

// QuoteName quotes database identifiers for Turso (same as SQLite)
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

// SupportsOperation checks if Turso supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "DROP_COLUMN", "RENAME_TABLE", "RENAME_COLUMN":
		return true
	case "ALTER_COLUMN":
		return false // Limited ALTER COLUMN support like SQLite
	default:
		return false
	}
}

// IsNotFoundError returns true when err is a Turso/SQLite "no such table/column/index" error.
func (p *Provider) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "no such table:") ||
		strings.HasPrefix(msg, "no such column:") ||
		strings.HasPrefix(msg, "no such index:")
}

// IsAlreadyExistsError returns true when err indicates an object already exists in the database.
func (p *Provider) IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ConvertFieldType converts YAML field type to Turso-specific SQL type (same as SQLite)
func (p *Provider) ConvertFieldType(field *types.Field) string {
	// Check user-defined type mappings first
	if p.typeMappings != nil {
		if mapping, ok := p.typeMappings[field.Type]; ok {
			resolved, err := typemap.ResolveType(mapping, field)
			if err == nil {
				return resolved
			}
			// Fall through to default on error
		}
	}

	switch field.Type {
	case "varchar":
		return "TEXT"
	case "text":
		return "TEXT"
	case "integer":
		return "INTEGER"
	case "bigint":
		return "INTEGER"
	case "serial":
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	case "float":
		return "REAL"
	case "decimal":
		return "REAL"
	case "boolean":
		return "INTEGER" // SQLite uses INTEGER for boolean
	case "date":
		return "TEXT"
	case "time":
		return "TEXT"
	case "timestamp":
		return "TEXT"
	case "uuid":
		return "TEXT"
	case "json", "jsonb":
		return "TEXT" // SQLite stores JSON as TEXT
	case "bytes":
		return "BLOB"
	default:
		return "TEXT"
	}
}

// GetDefaultValue converts default value references to Turso-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for Turso
func (p *Provider) GenerateCreateIndex(index *types.Index, tableName string) string {
	var quotedFields []string
	for _, fieldName := range index.Fields {
		quotedFields = append(quotedFields, p.QuoteName(fieldName))
	}

	indexType := ""
	if index.Unique {
		indexType = "UNIQUE "
	}

	return fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);",
		indexType,
		p.QuoteName(index.Name),
		p.QuoteName(tableName),
		strings.Join(quotedFields, ", "))
}

// GenerateDropIndex generates DROP INDEX statement for Turso
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("DROP INDEX %s;", p.QuoteName(indexName))
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates a DROP TABLE statement for Turso (libSQL/SQLite-compatible).
// Turso does not support CASCADE on DROP TABLE, so this is an alias for GenerateDropTable.
func (p *Provider) GenerateDropTableCascade(tableName string) string {
	return p.GenerateDropTable(tableName)
}

// GenerateAddColumn generates ALTER TABLE ADD COLUMN statement.
// The DEFAULT clause is emitted when field.Default is non-empty (already
// resolved from symbolic keys by resolveFieldDefault before this is called).
func (p *Provider) GenerateAddColumn(tableName string, field *types.Field) string {
	fieldDef := fmt.Sprintf("%s %s", p.QuoteName(field.Name), p.ConvertFieldType(field))

	if field.PrimaryKey {
		fieldDef += " PRIMARY KEY"
	}

	if field.AutoCreate && field.Type == "timestamp" {
		fieldDef += " DEFAULT CURRENT_TIMESTAMP"
	} else if field.Default != "" {
		fieldDef += " DEFAULT " + field.Default
	}

	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", p.QuoteName(tableName), fieldDef)
}

// GenerateDropColumn generates DROP COLUMN statement (newer SQLite/Turso feature)
func (p *Provider) GenerateDropColumn(tableName, columnName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", p.QuoteName(tableName), p.QuoteName(columnName))
}

// GenerateRenameTable generates ALTER TABLE RENAME statement
func (p *Provider) GenerateRenameTable(oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateRenameColumn generates RENAME COLUMN statement
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
		p.QuoteName(tableName), p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateCreateTable generates CREATE TABLE statement for Turso
func (p *Provider) GenerateCreateTable(schema *types.Schema, table *types.Table) (string, error) {
	var fieldDefs []string

	for _, field := range table.Fields {
		fieldDef, err := p.convertField(schema, &field)
		if err != nil {
			return "", fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		if fieldDef != "" {
			fieldDefs = append(fieldDefs, fieldDef)
		}
	}

	var sql strings.Builder
	fmt.Fprintf(&sql, "CREATE TABLE %s (\n", p.QuoteName(table.Name))

	for i, def := range fieldDefs {
		sql.WriteString("    " + def)
		if i < len(fieldDefs)-1 {
			sql.WriteString(",")
		}
		sql.WriteString("\n")
	}

	sql.WriteString(");")
	for i := range table.Indexes {
		sql.WriteString("\n")
		sql.WriteString(p.GenerateCreateIndex(&table.Indexes[i], table.Name))
	}

	return sql.String(), nil
}

func (p *Provider) convertField(schema *types.Schema, field *types.Field) (string, error) {
	if field.Type == "many_to_many" {
		return "", nil
	}

	var def strings.Builder
	def.WriteString(p.QuoteName(field.Name))
	def.WriteString(" ")

	// Handle serial/autoincrement specially
	if field.Type == "serial" {
		def.WriteString("INTEGER PRIMARY KEY AUTOINCREMENT")
		return def.String(), nil
	}

	sqlType := p.ConvertFieldType(field)
	def.WriteString(sqlType)

	if field.PrimaryKey && field.Type != "serial" {
		def.WriteString(" PRIMARY KEY")
	}

	if !field.IsNullable() && !field.PrimaryKey {
		def.WriteString(" NOT NULL")
	}

	// Handle auto_create for timestamp fields
	if field.AutoCreate && field.Type == "timestamp" {
		def.WriteString(" DEFAULT CURRENT_TIMESTAMP")
	} else if field.Default != "" {
		defaultValue := utils.ConvertDefaultValue(schema, "turso", field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: Turso (libSQL/SQLite) does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	return def.String(), nil
}

// Remaining interface methods
func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	oldType := p.ConvertFieldType(oldField)
	newType := p.ConvertFieldType(newField)

	if oldType == newType && oldField.IsNullable() == newField.IsNullable() &&
		oldField.Default == newField.Default &&
		oldField.AutoCreate == newField.AutoCreate {
		return "", nil
	}

	return "", fmt.Errorf("turso (libSQL) does not support ALTER COLUMN; use a RunSQL migration with table recreation (create new table, copy data, drop old, rename)")
}

func (p *Provider) GenerateForeignKeyConstraint(tableName, fieldName, referencedTable, constraintName, onDelete, onUpdate string) string {
	// Turso (libSQL) doesn't support ALTER TABLE ADD CONSTRAINT for foreign keys
	// FKs must be defined inline in CREATE TABLE
	return ""
}

func (p *Provider) GenerateDropForeignKeyConstraint(tableName, constraintName string) string {
	// Turso (libSQL) doesn't support ALTER TABLE DROP CONSTRAINT for foreign keys
	return ""
}

func (p *Provider) GenerateJunctionTable(table1, table2 string, schema *types.Schema) (string, error) {
	t1, t2 := table1, table2
	if t1 > t2 {
		t1, t2 = t2, t1
	}

	junctionName := fmt.Sprintf("%s_%s", t1, t2)
	col1 := fmt.Sprintf("%s_id", t1)
	col2 := fmt.Sprintf("%s_id", t2)

	fkType1 := p.InferForeignKeyType(t1, schema)
	fkType2 := p.InferForeignKeyType(t2, schema)

	return fmt.Sprintf("CREATE TABLE %s (\n    %s %s NOT NULL,\n    %s %s NOT NULL,\n    PRIMARY KEY (%s, %s),\n    FOREIGN KEY (%s) REFERENCES %s ON DELETE CASCADE,\n    FOREIGN KEY (%s) REFERENCES %s ON DELETE CASCADE\n);",
		p.QuoteName(junctionName),
		p.QuoteName(col1), fkType1,
		p.QuoteName(col2), fkType2,
		p.QuoteName(col1), p.QuoteName(col2),
		p.QuoteName(col1), p.QuoteName(t1),
		p.QuoteName(col2), p.QuoteName(t2),
	), nil
}

func (p *Provider) InferForeignKeyType(referencedTable string, schema *types.Schema) string {
	return "INTEGER"
}

func (p *Provider) GenerateIndexes(schema *types.Schema) string {
	var indexes []string

	for _, table := range schema.Tables {
		for _, field := range table.Fields {
			if field.Type == "foreign_key" {
				indexName := fmt.Sprintf("idx_%s_%s", table.Name, field.Name)
				indexSQL := fmt.Sprintf("CREATE INDEX %s ON %s (%s);",
					p.QuoteName(indexName),
					p.QuoteName(table.Name),
					p.QuoteName(field.Name))
				indexes = append(indexes, indexSQL)
			}
		}

		for _, index := range table.Indexes {
			indexSQL := p.GenerateCreateIndex(&index, table.Name)
			indexes = append(indexes, indexSQL)
		}
	}

	if len(indexes) == 0 {
		return ""
	}

	return strings.Join(indexes, "\n")
}

func (p *Provider) GenerateForeignKeyConstraints(schema *types.Schema, junctionTables []types.Table) string {
	var constraints []string

	for _, table := range schema.Tables {
		for _, field := range table.Fields {
			if field.Type == "foreign_key" && field.ForeignKey != nil {
				constraint := p.GenerateForeignKeyConstraint(table.Name, field.Name, field.ForeignKey.Table, "", field.ForeignKey.OnDelete, field.ForeignKey.OnUpdate)
				if constraint != "" {
					constraints = append(constraints, constraint)
				}
			}
		}
	}

	for _, junctionTable := range junctionTables {
		for _, field := range junctionTable.Fields {
			if field.Type == "foreign_key" && field.ForeignKey != nil {
				constraint := p.GenerateForeignKeyConstraint(junctionTable.Name, field.Name, field.ForeignKey.Table, "", field.ForeignKey.OnDelete, field.ForeignKey.OnUpdate)
				if constraint != "" {
					constraints = append(constraints, constraint)
				}
			}
		}
	}

	if len(constraints) == 0 {
		return ""
	}
	return strings.Join(constraints, "\n")
}

// GenerateUpsert generates an INSERT ... ON CONFLICT DO UPDATE SET statement for Turso.
// If all columns are conflict keys, it uses DO NOTHING instead.
// The valueLiterals are pre-formatted SQL literals and are not re-quoted.
func (p *Provider) GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string {
	if len(valueLiterals) == 0 {
		return ""
	}

	var sb strings.Builder

	// Quoted column names
	quotedCols := make([]string, len(columns))
	for i, c := range columns {
		quotedCols[i] = p.QuoteName(c)
	}

	// INSERT INTO "table" ("col1", "col2")
	fmt.Fprintf(&sb, "INSERT INTO %s (%s)\n", p.QuoteName(table), strings.Join(quotedCols, ", "))

	// VALUES rows
	for i, row := range valueLiterals {
		if i == 0 {
			fmt.Fprintf(&sb, "VALUES (%s)", strings.Join(row, ", "))
		} else {
			fmt.Fprintf(&sb, ",\n       (%s)", strings.Join(row, ", "))
		}
	}

	// ON CONFLICT clause
	quotedConflict := make([]string, len(conflictKeys))
	for i, k := range conflictKeys {
		quotedConflict[i] = p.QuoteName(k)
	}

	// Determine non-conflict columns for the UPDATE SET clause
	conflictSet := make(map[string]bool, len(conflictKeys))
	for _, k := range conflictKeys {
		conflictSet[k] = true
	}

	var updateCols []string
	for _, c := range columns {
		if !conflictSet[c] {
			updateCols = append(updateCols, c)
		}
	}

	if len(updateCols) == 0 {
		// All columns are conflict keys — use DO NOTHING
		fmt.Fprintf(&sb, "\nON CONFLICT(%s) DO NOTHING;", strings.Join(quotedConflict, ", "))
	} else {
		fmt.Fprintf(&sb, "\nON CONFLICT(%s) DO UPDATE SET\n", strings.Join(quotedConflict, ", "))
		for i, c := range updateCols {
			fmt.Fprintf(&sb, "  %s = excluded.%s", p.QuoteName(c), p.QuoteName(c))
			if i < len(updateCols)-1 {
				sb.WriteString(",\n")
			}
		}
		sb.WriteString(";")
	}

	return sb.String()
}

// TableColumns is not yet implemented for Turso.
func (p *Provider) TableColumns(db *sql.DB, tableName string) ([]string, error) {
	return nil, fmt.Errorf("TableColumns is not supported for Turso")
}

// GetDatabaseSchema extracts schema information from a Turso database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	return nil, fmt.Errorf("turso schema extraction not implemented yet")
}
