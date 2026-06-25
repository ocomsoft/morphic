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

// Package sqlserver provides a database provider for Microsoft SQL Server.
package sqlserver

import (
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
)

// Provider implements the Provider interface for SQL Server
type Provider struct {
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new SQL Server provider
func New() *Provider {
	return &Provider{}
}

// Placeholder returns the bind-parameter placeholder for the nth argument (1-indexed).
func (p *Provider) Placeholder(n int) string {
	return fmt.Sprintf("@p%d", n)
}

// HistoryTableDDL returns the CREATE TABLE IF NOT EXISTS statement for the
// morphic_history migration-tracking table, using this provider's SQL dialect.
func (p *Provider) HistoryTableDDL() string {
	return `IF NOT EXISTS (SELECT * FROM sysobjects WHERE name='morphic_history' AND xtype='U')
CREATE TABLE morphic_history (
    id INT IDENTITY(1,1) PRIMARY KEY,
    name NVARCHAR(255) NOT NULL UNIQUE,
    applied_at DATETIME2 DEFAULT CURRENT_TIMESTAMP
)`
}

// QuoteName quotes database identifiers for SQL Server
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf("[%s]", name)
}

// SupportsOperation checks if SQL Server supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "RENAME_TABLE", "RENAME_COLUMN", "DROP_COLUMN", "ALTER_COLUMN":
		return true
	default:
		return false
	}
}

// IsNotFoundError returns true when err is a SQL Server "does not exist" error
// (error codes 3701 / 4902).
func (p *Provider) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist or you do not have permission")
}

// IsAlreadyExistsError returns true when err indicates an object already exists in the database.
func (p *Provider) IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ConvertFieldType converts YAML field type to SQL Server-specific SQL type
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
		return "NVARCHAR(MAX)"
	case "text":
		return "NVARCHAR(MAX)"
	case "integer":
		return "INT"
	case "bigint":
		return "BIGINT"
	case "serial":
		return "INT IDENTITY(1,1)"
	case "float":
		return "FLOAT"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", field.Precision, field.Scale)
		}
		return "DECIMAL"
	case "boolean":
		return "BIT"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "timestamp":
		return "DATETIME2"
	case "uuid":
		return "UNIQUEIDENTIFIER"
	case "json", "jsonb":
		return "NVARCHAR(MAX)"
	case "bytes":
		return "VARBINARY(MAX)"
	default:
		return "NVARCHAR(MAX)"
	}
}

// GetDefaultValue converts default value references to SQL Server-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	// Return as literal value if not found in mapping
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for SQL Server.
// SQL Server supports WHERE clauses for filtered indexes but does not support Method (USING clause).
func (p *Provider) GenerateCreateIndex(index *types.Index, tableName string) string {
	var quotedFields []string
	for _, fieldName := range index.Fields {
		quotedFields = append(quotedFields, p.QuoteName(fieldName))
	}

	indexType := "INDEX"
	if index.Unique {
		indexType = "UNIQUE INDEX"
	}

	sql := fmt.Sprintf("CREATE %s %s ON %s (%s)",
		indexType,
		p.QuoteName(index.Name),
		p.QuoteName(tableName),
		strings.Join(quotedFields, ", "))

	if index.Where != "" {
		sql += fmt.Sprintf(" WHERE %s", index.Where)
	}

	return sql + ";"
}

// GenerateDropIndex generates DROP INDEX statement for SQL Server
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("DROP INDEX %s ON %s;", p.QuoteName(indexName), p.QuoteName(tableName))
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates a DROP TABLE statement for SQL Server.
// SQL Server does not support CASCADE on DROP TABLE, so this is an alias for GenerateDropTable.
func (p *Provider) GenerateDropTableCascade(tableName string) string {
	return p.GenerateDropTable(tableName)
}

// GenerateAddColumn generates ALTER TABLE ADD statement.
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

	return fmt.Sprintf("ALTER TABLE %s ADD %s;", p.QuoteName(tableName), fieldDef)
}

// GenerateDropColumn generates ALTER TABLE DROP COLUMN statement
func (p *Provider) GenerateDropColumn(tableName, columnName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", p.QuoteName(tableName), p.QuoteName(columnName))
}

// GenerateRenameTable generates sp_rename statement for table
func (p *Provider) GenerateRenameTable(oldName, newName string) string {
	return fmt.Sprintf("EXEC sp_rename '%s', '%s';", oldName, newName)
}

// GenerateRenameColumn generates sp_rename statement for column
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	return fmt.Sprintf("EXEC sp_rename '%s.%s', '%s', 'COLUMN';", tableName, oldName, newName)
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

	sql.WriteString(");")
	for i := range table.Indexes {
		sql.WriteString("\n")
		sql.WriteString(p.GenerateCreateIndex(&table.Indexes[i], table.Name))
	}

	return sql.String(), nil
}

// convertField converts a YAML field definition to SQL Server field definition
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
		def.WriteString(" DEFAULT GETDATE()")
	} else if field.Default != "" {
		// Convert default value using the schema's defaults mapping
		defaultValue := utils.ConvertDefaultValue(schema, "sqlserver", field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: SQL Server does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	// Generate primary key constraint if needed
	var constraint string
	if field.PrimaryKey {
		constraint = fmt.Sprintf("PRIMARY KEY (%s)", p.QuoteName(field.Name))
	}

	return def.String(), constraint, nil
}

// GenerateAlterColumn generates ALTER TABLE statements to modify a column definition in SQL Server.
func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	oldType := p.ConvertFieldType(oldField)
	newType := p.ConvertFieldType(newField)

	if oldType == newType && oldField.IsNullable() == newField.IsNullable() &&
		oldField.Default == newField.Default &&
		oldField.AutoCreate == newField.AutoCreate {
		return "", nil
	}

	var stmts []string
	tbl := p.QuoteName(tableName)
	col := p.QuoteName(newField.Name)

	// Type or nullability change
	if oldType != newType || oldField.IsNullable() != newField.IsNullable() {
		nullClause := " NULL"
		if !newField.IsNullable() {
			nullClause = " NOT NULL"
		}
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s%s;",
			tbl, col, newType, nullClause))
	}

	// Default change — SQL Server uses ADD/DROP CONSTRAINT for defaults
	if oldField.Default != newField.Default {
		if oldField.Default != "" {
			constraintName := fmt.Sprintf("DF_%s_%s", tableName, newField.Name)
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
				tbl, p.QuoteName(constraintName)))
		}
		if newField.Default != "" {
			constraintName := fmt.Sprintf("DF_%s_%s", tableName, newField.Name)
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s DEFAULT %s FOR %s;",
				tbl, p.QuoteName(constraintName), utils.FormatDefaultValue(newField.Default), col))
		}
	}

	// AutoCreate change — manages DEFAULT GETDATE() for timestamp fields
	if oldField.AutoCreate != newField.AutoCreate {
		constraintName := fmt.Sprintf("DF_%s_%s", tableName, newField.Name)
		if newField.AutoCreate && newField.Type == "timestamp" {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s DEFAULT GETDATE() FOR %s;",
				tbl, p.QuoteName(constraintName), col))
		} else if !newField.AutoCreate && oldField.AutoCreate {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;",
				tbl, p.QuoteName(constraintName)))
		}
	}

	// AutoUpdate: SQL Server does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	return strings.Join(stmts, "\n"), nil
}

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

// GenerateDropForeignKeyConstraint generates an ALTER TABLE DROP CONSTRAINT statement for SQL Server.
func (p *Provider) GenerateDropForeignKeyConstraint(tableName, constraintName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", p.QuoteName(tableName), p.QuoteName(constraintName))
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

// GenerateUpsert generates a MERGE INTO statement for SQL Server.
// If all columns are conflict keys, the WHEN MATCHED clause is omitted.
// The valueLiterals are pre-formatted SQL literals and are not re-quoted.
func (p *Provider) GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string {
	if len(valueLiterals) == 0 {
		return ""
	}

	var sb strings.Builder

	// Quoted column names for source alias
	quotedCols := make([]string, len(columns))
	for i, c := range columns {
		quotedCols[i] = p.QuoteName(c)
	}

	// MERGE INTO [table] AS target
	fmt.Fprintf(&sb, "MERGE INTO %s AS target\n", p.QuoteName(table))

	// USING (VALUES (...), (...)) AS source ([col1], [col2])
	sb.WriteString("USING (VALUES ")
	for i, row := range valueLiterals {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "(%s)", strings.Join(row, ", "))
	}
	fmt.Fprintf(&sb, ") AS source (%s)\n", strings.Join(quotedCols, ", "))

	// ON target.[k1] = source.[k1] AND ...
	var onClauses []string
	for _, k := range conflictKeys {
		qk := p.QuoteName(k)
		onClauses = append(onClauses, fmt.Sprintf("target.%s = source.%s", qk, qk))
	}
	fmt.Fprintf(&sb, "ON %s\n", strings.Join(onClauses, " AND "))

	// Determine non-conflict columns
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

	// WHEN MATCHED THEN UPDATE SET (only if there are non-key columns)
	if len(updateCols) > 0 {
		sb.WriteString("WHEN MATCHED THEN UPDATE SET ")
		for i, c := range updateCols {
			qc := p.QuoteName(c)
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "target.%s = source.%s", qc, qc)
		}
		sb.WriteString("\n")
	}

	// WHEN NOT MATCHED THEN INSERT
	var sourceCols []string
	for _, c := range columns {
		sourceCols = append(sourceCols, fmt.Sprintf("source.%s", p.QuoteName(c)))
	}
	fmt.Fprintf(&sb, "WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s);",
		strings.Join(quotedCols, ", "), strings.Join(sourceCols, ", "))

	return sb.String()
}

// GetDatabaseSchema extracts schema information from a SQL Server database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	return nil, fmt.Errorf("SQL Server schema extraction not implemented yet")
}
