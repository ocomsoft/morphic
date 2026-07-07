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
package redshift

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
)

// Provider implements the Provider interface for Amazon Redshift
// Redshift is based on PostgreSQL but has some differences
type Provider struct {
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new Redshift provider
func New() *Provider {
	return &Provider{}
}

// Placeholder returns the bind-parameter placeholder for the nth argument (1-indexed).
func (p *Provider) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

// HistoryTableDDL returns the CREATE TABLE IF NOT EXISTS statement for the
// morphic_history migration-tracking table, using this provider's SQL dialect.
func (p *Provider) HistoryTableDDL() string {
	return `CREATE TABLE IF NOT EXISTS morphic_history (
    id INTEGER IDENTITY(1,1) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
}

// QuoteName quotes database identifiers for Redshift (same as PostgreSQL)
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

// SupportsOperation checks if Redshift supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "RENAME_TABLE", "DROP_COLUMN", "ALTER_COLUMN":
		return true
	case "RENAME_COLUMN":
		// Redshift doesn't support RENAME COLUMN directly
		return false
	default:
		return false
	}
}

// IsNotFoundError returns true when err is a Redshift "does not exist" error.
func (p *Provider) IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "does not exist")
}

// IsAlreadyExistsError returns true when err indicates an object already exists in the database.
func (p *Provider) IsAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// ConvertFieldType converts YAML field type to Redshift-specific SQL type
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
		return "VARCHAR(65535)" // Redshift max VARCHAR size
	case "text":
		return "VARCHAR(65535)" // Redshift doesn't have unlimited TEXT type
	case "integer":
		return "INTEGER"
	case "bigint":
		return "BIGINT"
	case "serial":
		return "INTEGER IDENTITY(1,1)" // Redshift uses IDENTITY instead of SERIAL
	case "float":
		return "REAL"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", field.Precision, field.Scale)
		}
		return "DECIMAL(18,2)" // Default precision for Redshift
	case "boolean":
		return "BOOLEAN"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "timestamp":
		return "TIMESTAMP"
	case "uuid":
		return "VARCHAR(36)" // Redshift doesn't have native UUID type
	case "json", "jsonb":
		return "SUPER" // Redshift's native JSON type
	case "bytes":
		return "VARBINARY(65535)"
	default:
		return "VARCHAR(65535)"
	}
}

// GetDefaultValue converts default value references to Redshift-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	// Return as literal value if not found in mapping
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for Redshift
func (p *Provider) GenerateCreateIndex(index *types.Index, tableName string) string {
	var quotedFields []string
	for _, fieldName := range index.Fields {
		quotedFields = append(quotedFields, p.QuoteName(fieldName))
	}

	indexType := "INDEX"
	if index.Unique {
		indexType = "UNIQUE INDEX"
	}

	return fmt.Sprintf("CREATE %s %s ON %s (%s);",
		indexType,
		p.QuoteName(index.Name),
		p.QuoteName(tableName),
		strings.Join(quotedFields, ", "))
}

// GenerateDropIndex generates DROP INDEX statement for Redshift
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("DROP INDEX %s;", p.QuoteName(indexName))
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates a DROP TABLE statement for Redshift.
// Redshift does not support CASCADE on DROP TABLE in the same manner as PostgreSQL,
// so this is an alias for GenerateDropTable.
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

	if !field.IsNullable() {
		fieldDef += " NOT NULL"
	}

	if field.AutoCreate && field.Type == "timestamp" {
		fieldDef += " DEFAULT CURRENT_TIMESTAMP"
	} else if field.Default != "" {
		fieldDef += " DEFAULT " + field.Default
	}

	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", p.QuoteName(tableName), fieldDef)
}

// GenerateDropColumn generates ALTER TABLE DROP COLUMN statement
func (p *Provider) GenerateDropColumn(tableName, columnName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", p.QuoteName(tableName), p.QuoteName(columnName))
}

// GenerateRenameTable generates ALTER TABLE RENAME statement
func (p *Provider) GenerateRenameTable(oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateRenameColumn generates ALTER TABLE RENAME COLUMN statement
// Redshift doesn't support RENAME COLUMN directly, would need to use ADD/DROP pattern
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	// This would require a more complex implementation with ADD COLUMN + UPDATE + DROP COLUMN
	return fmt.Sprintf("-- Redshift doesn't support RENAME COLUMN. Use ADD COLUMN + UPDATE + DROP COLUMN pattern for %s.%s -> %s;",
		tableName, oldName, newName)
}

// GenerateCreateTable generates CREATE TABLE statement for Redshift
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

// convertField converts a YAML field definition to Redshift field definition
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
	if !field.IsNullable() || field.PrimaryKey {
		def.WriteString(" NOT NULL")
	}

	// Handle auto_create and auto_update for timestamp fields
	if field.AutoCreate && field.Type == "timestamp" {
		def.WriteString(" DEFAULT CURRENT_TIMESTAMP")
	} else if field.Default != "" {
		// Convert default value using the schema's defaults mapping
		defaultValue := utils.ConvertDefaultValue(schema, "redshift", field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: Redshift does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	// Generate primary key constraint if needed
	var constraint string
	if field.PrimaryKey {
		constraint = fmt.Sprintf("PRIMARY KEY (%s)", p.QuoteName(field.Name))
	}

	return def.String(), constraint, nil
}

// Placeholder implementations for remaining interface methods

func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	var stmts []string
	tbl := p.QuoteName(tableName)
	col := p.QuoteName(newField.Name)

	if p.ConvertFieldType(oldField) != p.ConvertFieldType(newField) {
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", tbl, col, p.ConvertFieldType(newField)))
	}
	if oldField.IsNullable() != newField.IsNullable() {
		if newField.IsNullable() {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tbl, col))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tbl, col))
		}
	}
	if oldField.Default != newField.Default {
		if newField.Default == "" {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tbl, col))
		} else {
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tbl, col, utils.FormatDefaultValue(newField.Default)))
		}
	}

	// AutoCreate change — manages DEFAULT CURRENT_TIMESTAMP for timestamp fields
	if oldField.AutoCreate != newField.AutoCreate {
		if newField.AutoCreate && newField.Type == "timestamp" {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT CURRENT_TIMESTAMP;",
				tbl, col))
		} else if !newField.AutoCreate && oldField.AutoCreate {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
				tbl, col))
		}
	}

	// AutoUpdate: Redshift does not support ON UPDATE natively.
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

func (p *Provider) GenerateDropForeignKeyConstraint(tableName, constraintName string) string {
	return fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", p.QuoteName(tableName), p.QuoteName(constraintName))
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
	return "BIGINT"
}

func (p *Provider) GenerateIndexes(schema *types.Schema) string {
	var indexes []string

	for _, table := range schema.Tables {
		// Generate indexes for foreign key fields
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

		// Generate table-level indexes (including unique indexes)
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

// GenerateUpsert generates a DELETE-then-INSERT pattern for Redshift, since Redshift
// does not support ON CONFLICT or MERGE. If no conflictKeys are provided, only the
// INSERT is generated. The valueLiterals are pre-formatted SQL literals and are not re-quoted.
func (p *Provider) GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string {
	if len(valueLiterals) == 0 {
		return ""
	}

	var sb strings.Builder

	quotedTable := p.QuoteName(table)

	// Quoted column names
	quotedCols := make([]string, len(columns))
	for i, c := range columns {
		quotedCols[i] = p.QuoteName(c)
	}

	// DELETE portion (only if conflictKeys are provided)
	if len(conflictKeys) > 0 {
		// Find the column indices for each conflict key
		colIndex := make(map[string]int, len(columns))
		for i, c := range columns {
			colIndex[c] = i
		}

		if len(conflictKeys) == 1 {
			// Single key: DELETE FROM "table" WHERE "key" IN (v1, v2, ...)
			idx := colIndex[conflictKeys[0]]
			var vals []string
			for _, row := range valueLiterals {
				vals = append(vals, row[idx])
			}
			fmt.Fprintf(&sb, "DELETE FROM %s WHERE %s IN (%s);\n",
				quotedTable, p.QuoteName(conflictKeys[0]), strings.Join(vals, ", "))
		} else {
			// Multiple keys: DELETE FROM "table" WHERE ("k1", "k2") IN ((v1a, v2a), (v1b, v2b))
			quotedConflict := make([]string, len(conflictKeys))
			for i, k := range conflictKeys {
				quotedConflict[i] = p.QuoteName(k)
			}

			var tuples []string
			for _, row := range valueLiterals {
				var vals []string
				for _, k := range conflictKeys {
					vals = append(vals, row[colIndex[k]])
				}
				tuples = append(tuples, fmt.Sprintf("(%s)", strings.Join(vals, ", ")))
			}
			fmt.Fprintf(&sb, "DELETE FROM %s WHERE (%s) IN (%s);\n",
				quotedTable, strings.Join(quotedConflict, ", "), strings.Join(tuples, ", "))
		}
	}

	// INSERT portion
	fmt.Fprintf(&sb, "INSERT INTO %s (%s)\n", quotedTable, strings.Join(quotedCols, ", "))
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

// TableColumns is not yet implemented for Redshift.
func (p *Provider) TableColumns(db *sql.DB, tableName string) ([]string, error) {
	return nil, fmt.Errorf("TableColumns is not supported for Redshift")
}

// GetDatabaseSchema extracts schema information from a Redshift database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	return nil, fmt.Errorf("redshift schema extraction not implemented yet")
}
