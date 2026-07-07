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

// Package mysql provides a database provider for MySQL and MariaDB.
package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
)

// Provider implements the Provider interface for MySQL
type Provider struct {
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new MySQL provider
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
    id INTEGER AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
}

// QuoteName quotes database identifiers for MySQL
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf("`%s`", name)
}

// SupportsOperation checks if MySQL supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "RENAME_TABLE", "DROP_COLUMN", "ALTER_COLUMN":
		return true
	case "RENAME_COLUMN":
		return true // MySQL uses CHANGE syntax
	default:
		return false
	}
}

// IsNotFoundError returns true when err is a MySQL "unknown table" or
// "can't drop key" error (codes 1051 and 1091).
func (p *Provider) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1051") || strings.Contains(msg, "Error 1091")
}

// IsAlreadyExistsError returns true when err indicates an object already exists in the database.
func (p *Provider) IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ConvertFieldType converts YAML field type to MySQL-specific SQL type
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
			return fmt.Sprintf("VARCHAR(%d)", field.Length)
		}
		return "TEXT"
	case "text":
		if field.Length > 0 {
			return fmt.Sprintf("VARCHAR(%d)", field.Length)
		}
		return "TEXT"
	case "integer":
		return "INT"
	case "bigint":
		return "BIGINT"
	case "serial":
		return "INT AUTO_INCREMENT"
	case "float":
		return "FLOAT"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", field.Precision, field.Scale)
		}
		return "DECIMAL"
	case "boolean":
		return "TINYINT(1)"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "timestamp":
		return "TIMESTAMP"
	case "uuid":
		return "CHAR(36)"
	case "json", "jsonb":
		return "JSON"
	case "bytes":
		return "BLOB"
	default:
		return "TEXT"
	}
}

// GetDefaultValue converts default value references to MySQL-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	// Return as literal value if not found in mapping
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for MySQL.
// MySQL supports Method (USING clause for index type) but does not support WHERE clauses.
func (p *Provider) GenerateCreateIndex(index *types.Index, tableName string) string {
	var quotedFields []string
	for _, fieldName := range index.Fields {
		quotedFields = append(quotedFields, p.QuoteName(fieldName))
	}

	indexType := "INDEX"
	if index.Unique {
		indexType = "UNIQUE INDEX"
	}

	sql := fmt.Sprintf("CREATE %s %s ON %s",
		indexType,
		p.QuoteName(index.Name),
		p.QuoteName(tableName))

	if index.Method != "" {
		sql += fmt.Sprintf(" USING %s", strings.ToUpper(index.Method))
	}

	sql += fmt.Sprintf(" (%s)", strings.Join(quotedFields, ", "))

	return sql + ";"
}

// GenerateDropIndex generates DROP INDEX statement for MySQL
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("DROP INDEX %s ON %s;", p.QuoteName(indexName), p.QuoteName(tableName))
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates DROP TABLE IF EXISTS statement for MySQL.
// MySQL does not support CASCADE on DROP TABLE; foreign key constraints must be dropped
// separately before dropping the table. IF EXISTS is used to prevent errors if the table
// does not exist, matching the behaviour callers expect for rollback scenarios.
func (p *Provider) GenerateDropTableCascade(tableName string) string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s;", p.QuoteName(tableName))
}

// GenerateAddColumn generates ALTER TABLE ADD COLUMN statement.
// The DEFAULT clause is emitted when field.Default is non-empty (already
// resolved from symbolic keys by resolveFieldDefault before this is called).
func (p *Provider) GenerateAddColumn(tableName string, field *types.Field) string {
	fieldDef := fmt.Sprintf("%s %s", p.QuoteName(field.Name), p.ConvertFieldType(field))

	if field.PrimaryKey {
		fieldDef += " PRIMARY KEY"
	}

	if !field.IsNullable() {
		fieldDef += " NOT NULL"
	}

	if field.AutoCreate && field.Type == "timestamp" {
		fieldDef += " DEFAULT CURRENT_TIMESTAMP"
	} else if field.Default != "" {
		fieldDef += " DEFAULT " + field.Default
	}

	// AutoUpdate: MySQL supports ON UPDATE CURRENT_TIMESTAMP natively
	if field.AutoUpdate && field.Type == "timestamp" {
		fieldDef += " ON UPDATE CURRENT_TIMESTAMP"
	}

	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", p.QuoteName(tableName), fieldDef)
}

// GenerateDropColumn generates ALTER TABLE DROP COLUMN statement
func (p *Provider) GenerateDropColumn(tableName, columnName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", p.QuoteName(tableName), p.QuoteName(columnName))
}

// GenerateRenameTable generates RENAME TABLE statement
func (p *Provider) GenerateRenameTable(oldName, newName string) string {
	return fmt.Sprintf("RENAME TABLE %s TO %s;", p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateRenameColumn generates ALTER TABLE CHANGE statement for MySQL
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	// MySQL uses CHANGE syntax which requires the full column definition
	// This is a simplified version - would need field definition in real implementation
	return fmt.Sprintf("ALTER TABLE %s CHANGE %s %s VARCHAR(255);",
		p.QuoteName(tableName), p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateCreateTable generates the CREATE TABLE SQL statement for the given table.
func (p *Provider) GenerateCreateTable(schema *types.Schema, table *types.Table) (string, error) {
	var fieldDefs []string
	var constraints []string

	for _, field := range table.Fields {
		fieldDef, constraint, err := p.convertField(schema, &field)
		if err != nil {
			return "", fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		// Only add non-empty field definitions (skip many_to_many fields)
		if fieldDef != "" {
			fieldDefs = append(fieldDefs, fieldDef)
		}
		if constraint != "" {
			constraints = append(constraints, constraint)
		}
	}

	// Combine field definitions and constraints
	allDefs := append(fieldDefs, constraints...)

	// Build CREATE TABLE statement
	var sql strings.Builder
	fmt.Fprintf(&sql, "CREATE TABLE %s (\n", p.QuoteName(table.Name))

	for i, def := range allDefs {
		sql.WriteString("    " + def)
		if i < len(allDefs)-1 {
			sql.WriteString(",")
		}
		sql.WriteString("\n")
	}

	sql.WriteString(") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;")
	for i := range table.Indexes {
		sql.WriteString("\n")
		sql.WriteString(p.GenerateCreateIndex(&table.Indexes[i], table.Name))
	}

	return sql.String(), nil
}

// convertField converts a YAML field definition to MySQL field definition
func (p *Provider) convertField(schema *types.Schema, field *types.Field) (string, string, error) {
	// Skip many_to_many fields - they don't create actual columns
	if field.Type == "many_to_many" {
		return "", "", nil
	}

	var def strings.Builder
	def.WriteString(p.QuoteName(field.Name))
	def.WriteString(" ")

	// Convert field type
	sqlType := p.ConvertFieldType(field)
	def.WriteString(sqlType)

	// Add NOT NULL constraint
	if !field.IsNullable() {
		def.WriteString(" NOT NULL")
	}

	// Handle auto_create and auto_update for timestamp fields
	if field.AutoCreate && field.Type == "timestamp" {
		def.WriteString(" DEFAULT CURRENT_TIMESTAMP")
	} else if field.Default != "" {
		// Convert default value using the schema's defaults mapping
		defaultValue := utils.ConvertDefaultValue(schema, "mysql", field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: MySQL supports ON UPDATE CURRENT_TIMESTAMP natively
	if field.AutoUpdate && field.Type == "timestamp" {
		def.WriteString(" ON UPDATE CURRENT_TIMESTAMP")
	}

	// Generate primary key constraint if needed
	var constraint string
	if field.PrimaryKey {
		constraint = fmt.Sprintf("PRIMARY KEY (%s)", p.QuoteName(field.Name))
	}

	return def.String(), constraint, nil
}

// GenerateAlterColumn generates an ALTER TABLE MODIFY COLUMN statement for MySQL.
func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	oldType := p.ConvertFieldType(oldField)
	newType := p.ConvertFieldType(newField)

	// MySQL uses MODIFY COLUMN which requires the full column definition.
	// Only emit a statement if something actually changed.
	if oldType == newType && oldField.IsNullable() == newField.IsNullable() &&
		oldField.Default == newField.Default &&
		oldField.AutoCreate == newField.AutoCreate &&
		oldField.AutoUpdate == newField.AutoUpdate {
		return "", nil
	}

	tbl := p.QuoteName(tableName)
	col := p.QuoteName(newField.Name)

	stmt := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", tbl, col, newType)
	if !newField.IsNullable() {
		stmt += " NOT NULL"
	}
	// AutoCreate: set DEFAULT CURRENT_TIMESTAMP for timestamp fields
	if newField.AutoCreate && newField.Type == "timestamp" {
		stmt += " DEFAULT CURRENT_TIMESTAMP"
	} else if newField.Default != "" {
		stmt += fmt.Sprintf(" DEFAULT %s", utils.FormatDefaultValue(newField.Default))
	}

	// AutoUpdate: MySQL supports ON UPDATE CURRENT_TIMESTAMP natively
	if newField.AutoUpdate && newField.Type == "timestamp" {
		stmt += " ON UPDATE CURRENT_TIMESTAMP"
	}

	stmt += ";"

	return stmt, nil
}

// GenerateForeignKeyConstraint generates an ALTER TABLE statement to add a foreign key constraint.
func (p *Provider) GenerateForeignKeyConstraint(tableName, fieldName, referencedTable, constraintName, onDelete, onUpdate string) string {
	if constraintName == "" {
		constraintName = fmt.Sprintf("fk_%s_%s", tableName, fieldName)
	}
	onDeleteClause := ""
	if onDelete != "" {
		onDeleteClause = fmt.Sprintf(" ON DELETE %s", strings.ToUpper(onDelete))
	}
	onUpdateClause := ""
	if onUpdate != "" {
		onUpdateClause = fmt.Sprintf(" ON UPDATE %s", strings.ToUpper(onUpdate))
	}
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s%s%s;",
		p.QuoteName(tableName), p.QuoteName(constraintName), p.QuoteName(fieldName), p.QuoteName(referencedTable), onDeleteClause, onUpdateClause)
}

// GenerateDropForeignKeyConstraint generates an ALTER TABLE DROP FOREIGN KEY statement for MySQL.
func (p *Provider) GenerateDropForeignKeyConstraint(tableName, constraintName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;", p.QuoteName(tableName), p.QuoteName(constraintName))
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

	return fmt.Sprintf("CREATE TABLE %s (\n    %s %s NOT NULL,\n    %s %s NOT NULL,\n    PRIMARY KEY (%s, %s),\n    FOREIGN KEY (%s) REFERENCES %s ON DELETE CASCADE,\n    FOREIGN KEY (%s) REFERENCES %s ON DELETE CASCADE\n);",
		p.QuoteName(junctionName),
		p.QuoteName(col1), fkType1,
		p.QuoteName(col2), fkType2,
		p.QuoteName(col1), p.QuoteName(col2),
		p.QuoteName(col1), p.QuoteName(t1),
		p.QuoteName(col2), p.QuoteName(t2),
	), nil
}

// InferForeignKeyType returns the SQL type to use for a foreign key column referencing the given table.
func (p *Provider) InferForeignKeyType(referencedTable string, schema *types.Schema) string {
	return "BIGINT"
}

// GenerateIndexes generates CREATE INDEX statements for all tables in the schema.
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

// GenerateForeignKeyConstraints generates all foreign key constraint SQL statements for a schema.
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

// TableColumns is not yet implemented for MySQL.
func (p *Provider) TableColumns(db *sql.DB, tableName string) ([]string, error) {
	return nil, fmt.Errorf("TableColumns is not supported for MySQL")
}

// GetDatabaseSchema extracts schema information from a MySQL database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	return nil, fmt.Errorf("MySQL schema extraction not implemented yet")
}

// GenerateUpsert generates a MySQL INSERT ... ON DUPLICATE KEY UPDATE statement.
// MySQL identifies conflicts automatically via unique indexes, so conflictKeys is
// accepted for interface compatibility but not used in the generated SQL.
func (p *Provider) GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string {
	if len(valueLiterals) == 0 {
		return ""
	}

	quotedCols := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = p.QuoteName(col)
	}

	var rows []string
	for _, vals := range valueLiterals {
		rows = append(rows, "("+strings.Join(vals, ", ")+")")
	}

	var updates []string
	for _, col := range columns {
		qc := p.QuoteName(col)
		updates = append(updates, fmt.Sprintf("  %s = VALUES(%s)", qc, qc))
	}

	return fmt.Sprintf("INSERT INTO %s (%s)\nVALUES %s\nON DUPLICATE KEY UPDATE\n%s;",
		p.QuoteName(table),
		strings.Join(quotedCols, ", "),
		strings.Join(rows, ",\n       "),
		strings.Join(updates, ",\n"),
	)
}
