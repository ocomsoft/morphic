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

// Package clickhouse provides a database provider for ClickHouse columnar databases.
package clickhouse

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
)

// Provider implements the Provider interface for ClickHouse
// ClickHouse has very different SQL syntax and concepts from other databases
type Provider struct {
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new ClickHouse provider
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
    name String NOT NULL,
    applied_at String DEFAULT toString(now())
) ENGINE = ReplacingMergeTree() ORDER BY name`
}

// QuoteName quotes database identifiers for ClickHouse (backticks like MySQL)
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf("`%s`", name)
}

// SupportsOperation checks if ClickHouse supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "DROP_COLUMN", "ALTER_COLUMN":
		return true
	case "RENAME_TABLE", "RENAME_COLUMN":
		// ClickHouse supports RENAME TABLE but it's different syntax
		return false
	default:
		return false
	}
}

// IsNotFoundError returns true when err is a ClickHouse "doesn't exist" error.
func (p *Provider) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "doesn't exist")
}

// IsAlreadyExistsError returns true when err indicates an object already exists in the database.
func (p *Provider) IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ConvertFieldType converts YAML field type to ClickHouse-specific SQL type
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
		if field.Length > 0 {
			return fmt.Sprintf("FixedString(%d)", field.Length)
		}
		return "String"
	case "text":
		return "String"
	case "integer":
		return "Int32"
	case "bigint":
		return "Int64"
	case "serial":
		return "UInt64" // ClickHouse doesn't have auto-increment, typically use UInt64
	case "float":
		return "Float32"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("Decimal(%d,%d)", field.Precision, field.Scale)
		}
		return "Decimal(18,2)"
	case "boolean":
		return "UInt8" // ClickHouse uses UInt8 for boolean (0/1)
	case "date":
		return "Date"
	case "time":
		return "DateTime" // ClickHouse doesn't have separate TIME type
	case "timestamp":
		return "DateTime"
	case "uuid":
		return "UUID"
	case "json", "jsonb":
		return "String" // ClickHouse doesn't have native JSON, store as String
	case "bytes":
		return "String"
	default:
		return "String"
	}
}

// GetDefaultValue converts default value references to ClickHouse-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	// Return as literal value if not found in mapping
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for ClickHouse
// ClickHouse doesn't have traditional indexes, but has skip indexes and primary keys
func (p *Provider) GenerateCreateIndex(index *types.Index, tableName string) string {
	// ClickHouse doesn't support traditional CREATE INDEX
	// This would need to be implemented as a skip index or handled during table creation
	var quotedFields []string
	for _, fieldName := range index.Fields {
		quotedFields = append(quotedFields, p.QuoteName(fieldName))
	}

	// Return a comment explaining this limitation
	return fmt.Sprintf("-- ClickHouse doesn't support CREATE INDEX. Consider using skip indexes or include in PRIMARY KEY during table creation for %s on %s (%s);",
		index.Name, tableName, strings.Join(quotedFields, ", "))
}

// GenerateDropIndex generates DROP INDEX statement for ClickHouse
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("-- ClickHouse doesn't support DROP INDEX for %s on %s;", indexName, tableName)
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates a DROP TABLE statement for ClickHouse.
// ClickHouse does not support CASCADE on DROP TABLE, so this is an alias for GenerateDropTable.
func (p *Provider) GenerateDropTableCascade(tableName string) string {
	return p.GenerateDropTable(tableName)
}

// GenerateAddColumn generates ALTER TABLE ADD COLUMN statement
func (p *Provider) GenerateAddColumn(tableName string, field *types.Field) string {
	fieldDef := fmt.Sprintf("%s %s", p.QuoteName(field.Name), p.ConvertFieldType(field))

	// ClickHouse ADD COLUMN syntax
	if field.AutoCreate && field.Type == "timestamp" {
		fieldDef += " DEFAULT now()"
	} else if field.Default != "" {
		fieldDef += fmt.Sprintf(" DEFAULT %s", field.Default)
	}

	// AutoUpdate: ClickHouse does not support ON UPDATE natively.

	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", p.QuoteName(tableName), fieldDef)

	// ClickHouse PRIMARY KEY is defined at table level (ENGINE clause), not inline on columns.
	// Adding a PK column via ALTER TABLE ADD COLUMN requires recreating the table.
	if field.PrimaryKey {
		sql = fmt.Sprintf("-- ClickHouse does not support adding PRIMARY KEY columns via ALTER TABLE. Recreate the table to add %s as a PRIMARY KEY column.\n%s",
			p.QuoteName(field.Name), sql)
	}

	return sql
}

// GenerateDropColumn generates ALTER TABLE DROP COLUMN statement
func (p *Provider) GenerateDropColumn(tableName, columnName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", p.QuoteName(tableName), p.QuoteName(columnName))
}

// GenerateRenameTable generates RENAME TABLE statement
func (p *Provider) GenerateRenameTable(oldName, newName string) string {
	return fmt.Sprintf("RENAME TABLE %s TO %s;", p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateRenameColumn generates ALTER TABLE RENAME COLUMN statement
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
		p.QuoteName(tableName), p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateCreateTable generates CREATE TABLE statement for ClickHouse
func (p *Provider) GenerateCreateTable(schema *types.Schema, table *types.Table) (string, error) {
	var fieldDefs []string
	var primaryKeys []string

	for _, field := range table.Fields {
		fieldDef, err := p.convertField(schema, &field)
		if err != nil {
			return "", fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		// Only add non-empty field definitions (skip many_to_many fields)
		if fieldDef != "" {
			fieldDefs = append(fieldDefs, fieldDef)
		}

		// Collect primary key fields
		if field.PrimaryKey {
			primaryKeys = append(primaryKeys, p.QuoteName(field.Name))
		}
	}

	// Build CREATE TABLE statement
	var sql strings.Builder
	fmt.Fprintf(&sql, "CREATE TABLE %s (\n", p.QuoteName(table.Name))

	for i, def := range fieldDefs {
		sql.WriteString("    " + def)
		if i < len(fieldDefs)-1 {
			sql.WriteString(",")
		}
		sql.WriteString("\n")
	}

	sql.WriteString(")")

	// ClickHouse requires an ENGINE clause
	// Default to MergeTree with primary key if available, otherwise use Log
	if len(primaryKeys) > 0 {
		fmt.Fprintf(&sql, "\nENGINE = MergeTree()\nPRIMARY KEY (%s)", strings.Join(primaryKeys, ", "))
	} else {
		sql.WriteString("\nENGINE = Log()")
	}

	sql.WriteString(";")
	for i := range table.Indexes {
		sql.WriteString("\n")
		sql.WriteString(p.GenerateCreateIndex(&table.Indexes[i], table.Name))
	}

	return sql.String(), nil
}

// convertField converts a YAML field definition to ClickHouse field definition
func (p *Provider) convertField(schema *types.Schema, field *types.Field) (string, error) {
	// Skip many_to_many fields - they don't create actual columns
	if field.Type == "many_to_many" {
		return "", nil
	}

	var def strings.Builder
	def.WriteString(p.QuoteName(field.Name))
	def.WriteString(" ")

	// Convert field type
	sqlType := p.ConvertFieldType(field)
	def.WriteString(sqlType)

	// ClickHouse doesn't have NULL/NOT NULL in the same way as other databases
	// All columns are NOT NULL by default unless you use Nullable(Type)
	if field.IsNullable() && !field.PrimaryKey {
		// Reset and rebuild with Nullable wrapper
		def.Reset()
		def.WriteString(p.QuoteName(field.Name))
		def.WriteString(" Nullable(")
		def.WriteString(sqlType)
		def.WriteString(")")
	}

	// Handle defaults
	if field.AutoCreate && field.Type == "timestamp" {
		def.WriteString(" DEFAULT now()")
	} else if field.Default != "" {
		defaultValue := utils.ConvertDefaultValue(schema, "clickhouse", field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: ClickHouse does not support ON UPDATE natively.

	return def.String(), nil
}

// GenerateAlterColumn generates an ALTER TABLE MODIFY COLUMN statement for ClickHouse.
func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	oldType := p.ConvertFieldType(oldField)
	newType := p.ConvertFieldType(newField)

	// ClickHouse has no NOT NULL concept, so only check type, default, and AutoCreate
	if oldType == newType && oldField.Default == newField.Default &&
		oldField.AutoCreate == newField.AutoCreate {
		return "", nil
	}

	tbl := p.QuoteName(tableName)
	col := p.QuoteName(newField.Name)

	stmt := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", tbl, col, newType)
	// AutoCreate: set DEFAULT now() for timestamp fields
	if newField.AutoCreate && newField.Type == "timestamp" {
		stmt += " DEFAULT now()"
	} else if newField.Default != "" {
		stmt += fmt.Sprintf(" DEFAULT %s", utils.FormatDefaultValue(newField.Default))
	}

	// AutoUpdate: ClickHouse does not support ON UPDATE natively.

	stmt += ";"

	return stmt, nil
}

// GenerateForeignKeyConstraint returns a no-op comment because ClickHouse does not support foreign keys.
func (p *Provider) GenerateForeignKeyConstraint(tableName, fieldName, referencedTable, constraintName, onDelete, onUpdate string) string {
	// ClickHouse doesn't support foreign keys
	return fmt.Sprintf("-- ClickHouse doesn't support foreign key constraints for %s.%s -> %s;", tableName, fieldName, referencedTable)
}

// GenerateDropForeignKeyConstraint returns a no-op comment because ClickHouse does not support foreign keys.
func (p *Provider) GenerateDropForeignKeyConstraint(tableName, constraintName string) string {
	return fmt.Sprintf("-- ClickHouse doesn't support foreign key constraints for %s.%s;", tableName, constraintName)
}

// GenerateJunctionTable generates the CREATE TABLE SQL for a many-to-many junction table.
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

	return fmt.Sprintf("CREATE TABLE %s (\n    %s %s,\n    %s %s\n)\nENGINE = MergeTree()\nPRIMARY KEY (%s, %s);",
		p.QuoteName(junctionName),
		p.QuoteName(col1), fkType1,
		p.QuoteName(col2), fkType2,
		p.QuoteName(col1), p.QuoteName(col2),
	), nil
}

// InferForeignKeyType returns the SQL type to use for a foreign key column referencing the given table.
func (p *Provider) InferForeignKeyType(referencedTable string, schema *types.Schema) string {
	return "UInt64" // Default to UInt64 for foreign keys in ClickHouse
}

// GenerateIndexes generates skip-index comments for all tables in the schema.
func (p *Provider) GenerateIndexes(schema *types.Schema) string {
	var comments []string

	for _, table := range schema.Tables {
		// Generate comments for foreign key fields
		for _, field := range table.Fields {
			if field.Type == "foreign_key" {
				comment := fmt.Sprintf("-- ClickHouse doesn't support indexes. Consider using skip indexes for %s.%s;", table.Name, field.Name)
				comments = append(comments, comment)
			}
		}

		// Generate comments for table-level indexes
		for _, index := range table.Indexes {
			comment := p.GenerateCreateIndex(&index, table.Name)
			comments = append(comments, comment)
		}
	}

	if len(comments) == 0 {
		return ""
	}

	return strings.Join(comments, "\n")
}

// GenerateForeignKeyConstraints returns a no-op comment because ClickHouse does not support foreign keys.
func (p *Provider) GenerateForeignKeyConstraints(schema *types.Schema, junctionTables []types.Table) string {
	return "-- ClickHouse doesn't support foreign key constraints;"
}

// GenerateUpsert generates a plain INSERT INTO statement for ClickHouse. ClickHouse does not
// support traditional upsert; deduplication is handled by the table engine (e.g. ReplacingMergeTree).
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

	sb.WriteString("-- Note: ClickHouse deduplication is handled by the table engine (e.g. ReplacingMergeTree)\n")
	fmt.Fprintf(&sb, "INSERT INTO %s (%s)\n", p.QuoteName(table), strings.Join(quotedCols, ", "))

	for i, row := range valueLiterals {
		if i == 0 {
			fmt.Fprintf(&sb, "VALUES (%s)", strings.Join(row, ", "))
		} else {
			fmt.Fprintf(&sb, ",\n       (%s)", strings.Join(row, ", "))
		}
	}
	sb.WriteString(";")

	return sb.String()
}

// TableColumns is not yet implemented for ClickHouse.
func (p *Provider) TableColumns(db *sql.DB, tableName string) ([]string, error) {
	return nil, fmt.Errorf("TableColumns is not supported for ClickHouse")
}

// GetDatabaseSchema extracts schema information from a ClickHouse database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	return nil, fmt.Errorf("ClickHouse schema extraction not implemented yet")
}
