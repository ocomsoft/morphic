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

// Package postgresql provides a database provider for PostgreSQL.
package postgresql

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/ocomsoft/morphic/internal/fkutils"
	"github.com/ocomsoft/morphic/internal/typemap"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/utils"
	"github.com/ocomsoft/morphic/internal/version"
)

// Provider implements the Provider interface for PostgreSQL
type Provider struct {
	fkResolver   *fkutils.ForeignKeyTypeResolver
	typeMappings map[string]string
}

// SetTypeMappings sets user-defined type mappings for this provider.
func (p *Provider) SetTypeMappings(mappings map[string]string) {
	p.typeMappings = mappings
}

// New creates a new PostgreSQL provider
func New() *Provider {
	return &Provider{
		fkResolver: &fkutils.ForeignKeyTypeResolver{
			UUIDType:    "UUID",
			IntegerType: "INTEGER",
			BigIntType:  "BIGINT",
			SerialType:  "INTEGER",
		},
	}
}

// QuoteName quotes database identifiers for PostgreSQL
func (p *Provider) QuoteName(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

// SupportsOperation checks if PostgreSQL supports a specific operation
func (p *Provider) SupportsOperation(operation string) bool {
	switch operation {
	case "RENAME_COLUMN", "RENAME_TABLE", "DROP_COLUMN", "ALTER_COLUMN":
		return true
	default:
		return false
	}
}

// Placeholder returns the bind-parameter placeholder for the nth argument (1-indexed).
func (p *Provider) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

// HistoryTableDDL returns the CREATE TABLE IF NOT EXISTS statement for the
// morphic_history migration-tracking table, using this provider's SQL dialect.
func (p *Provider) HistoryTableDDL() string {
	return `CREATE TABLE IF NOT EXISTS morphic_history (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
)`
}

// IsNotFoundError returns true when err is a PostgreSQL "does not exist" error.
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

// ConvertFieldType converts YAML field type to PostgreSQL-specific SQL type
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
		return "TEXT"
	case "integer":
		return "INTEGER"
	case "bigint":
		return "BIGINT"
	case "serial":
		return "SERIAL"
	case "float":
		return "REAL"
	case "decimal":
		if field.Precision > 0 && field.Scale >= 0 {
			return fmt.Sprintf("DECIMAL(%d,%d)", field.Precision, field.Scale)
		}
		return "DECIMAL"
	case "boolean":
		return "BOOLEAN"
	case "date":
		return "DATE"
	case "time":
		return "TIME"
	case "timestamp":
		return "TIMESTAMP"
	case "uuid":
		return "UUID"
	case "json", "jsonb":
		return "JSONB"
	case "bytes":
		return "BYTEA"
	case "foreign_key":
		// Foreign keys default to UUID for PostgreSQL
		// The actual type will be determined in convertField based on the referenced table
		return "UUID"
	default:
		return strings.ToUpper(field.Type)
	}
}

// ConvertFieldTypeWithSchema converts YAML field type to PostgreSQL-specific SQL type with schema context for foreign keys
func (p *Provider) ConvertFieldTypeWithSchema(schema *types.Schema, field *types.Field) string {
	if field.Type == "foreign_key" && field.ForeignKey != nil {
		// Look up the referenced table's primary key type
		return p.getForeignKeyType(schema, field.ForeignKey.Table)
	}
	return p.ConvertFieldType(field)
}

// getForeignKeyType determines the appropriate SQL type for a foreign key field
// by looking up the referenced table's primary key type using the shared resolver
func (p *Provider) getForeignKeyType(schema *types.Schema, referencedTableName string) string {
	return p.fkResolver.GetForeignKeyType(schema, referencedTableName)
}

// GetDefaultValue converts default value references to PostgreSQL-specific values
func (p *Provider) GetDefaultValue(defaultRef string, defaults map[string]string) (string, error) {
	if value, exists := defaults[defaultRef]; exists {
		return value, nil
	}
	// Return as literal value if not found in mapping
	return fmt.Sprintf("'%s'", defaultRef), nil
}

// GenerateCreateIndex generates CREATE INDEX statement for PostgreSQL
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
	if index.Where != "" {
		sql += fmt.Sprintf(" WHERE %s", index.Where)
	}
	return sql + ";"
}

// GenerateDropIndex generates DROP INDEX statement for PostgreSQL
func (p *Provider) GenerateDropIndex(indexName, tableName string) string {
	return fmt.Sprintf("DROP INDEX %s;", p.QuoteName(indexName))
}

// GenerateDropTable generates DROP TABLE statement
func (p *Provider) GenerateDropTable(tableName string) string {
	return fmt.Sprintf("DROP TABLE %s;", p.QuoteName(tableName))
}

// GenerateDropTableCascade generates DROP TABLE IF EXISTS ... CASCADE statement for PostgreSQL.
// The CASCADE clause automatically drops any dependent objects (such as foreign key constraints
// from other tables referencing this one), preventing ordering failures during rollback.
func (p *Provider) GenerateDropTableCascade(tableName string) string {
	return fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", p.QuoteName(tableName))
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
		fieldDef += " DEFAULT " + p.convertDefaultValue(nil, field.Default)
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
func (p *Provider) GenerateRenameColumn(tableName, oldName, newName string) string {
	return fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
		p.QuoteName(tableName), p.QuoteName(oldName), p.QuoteName(newName))
}

// GenerateCreateTable generates the CREATE TABLE SQL statement for the given table.
func (p *Provider) GenerateCreateTable(schema *types.Schema, table *types.Table) (string, error) {
	var fieldDefs []string
	var constraints []string

	// Pre-scan to collect all PK field names so we can emit a single composite
	// PRIMARY KEY constraint instead of one per field (PostgreSQL allows only one).
	var pkFields []string
	for _, field := range table.Fields {
		if field.PrimaryKey {
			pkFields = append(pkFields, p.QuoteName(field.Name))
		}
	}

	for _, field := range table.Fields {
		fieldDef, constraint, err := p.convertField(schema, &field)
		if err != nil {
			return "", fmt.Errorf("failed to convert field %s: %w", field.Name, err)
		}

		// Only add non-empty field definitions (skip many_to_many fields)
		if fieldDef != "" {
			fieldDefs = append(fieldDefs, fieldDef)
		}
		// Suppress per-field PRIMARY KEY constraints when there are multiple PK
		// columns; the composite constraint is added below.
		if constraint != "" && len(pkFields) <= 1 {
			constraints = append(constraints, constraint)
		}
	}

	// Emit a single composite PRIMARY KEY when multiple PK columns exist.
	if len(pkFields) > 1 {
		constraints = append(constraints, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkFields, ", ")))
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

// convertField converts a YAML field definition to PostgreSQL field definition
func (p *Provider) convertField(schema *types.Schema, field *types.Field) (string, string, error) {
	// Skip many_to_many fields - they don't create actual columns
	if field.Type == "many_to_many" {
		return "", "", nil
	}

	var def strings.Builder
	def.WriteString(p.QuoteName(field.Name))
	def.WriteString(" ")

	// Convert field type with schema context for foreign keys
	sqlType := p.ConvertFieldTypeWithSchema(schema, field)
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
		defaultValue := p.convertDefaultValue(schema, field.Default)
		def.WriteString(" DEFAULT " + defaultValue)
	}

	// AutoUpdate: PostgreSQL does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	// Generate primary key constraint if needed
	var constraint string
	if field.PrimaryKey {
		constraint = fmt.Sprintf("PRIMARY KEY (%s)", p.QuoteName(field.Name))
	}

	return def.String(), constraint, nil
}

// isSQLExpression returns true when the value is already a valid SQL expression
// that should be emitted verbatim rather than wrapped in single quotes.
// This covers values produced by resolveFieldDefault (symbolic key → SQL expression):
//   - Single-quoted string literals already in SQL form: 'value', '[]'::jsonb
//   - SQL function calls: gen_random_uuid(), uuid_generate_v4()
//   - Type cast expressions: '{}'::jsonb
//   - SQL keywords (all-uppercase, letters and underscores): CURRENT_TIMESTAMP
//   - NULL/null, TRUE/true, FALSE/false literals
func isSQLExpression(value string) bool {
	if value == "" {
		return false
	}
	// Already-quoted SQL string literal
	if strings.HasPrefix(value, "'") {
		return true
	}
	// SQL function call or expression containing parentheses
	if strings.Contains(value, "(") {
		return true
	}
	// Type cast expression (e.g. '[]'::jsonb)
	if strings.Contains(value, "::") {
		return true
	}
	// SQL null / boolean literals
	lower := strings.ToLower(value)
	if lower == "null" || lower == "true" || lower == "false" {
		return true
	}
	// SQL keyword: all uppercase ASCII letters and underscores (e.g. CURRENT_TIMESTAMP)
	if len(value) > 1 {
		allUpper := true
		for _, ch := range value {
			if (ch < 'A' || ch > 'Z') && ch != '_' {
				allUpper = false
				break
			}
		}
		if allUpper {
			return true
		}
	}
	return false
}

// convertDefaultValue converts a default value to PostgreSQL-specific SQL.
// Resolution order:
//  1. Look up symbolic key in schema.Defaults for PostgreSQL (if mapping is present).
//  2. If the value is already a SQL expression (function call, keyword, etc.), use it as-is.
//  3. If the value is numeric, use it as-is.
//  4. Otherwise, wrap it in single quotes as a string literal.
func (p *Provider) convertDefaultValue(schema *types.Schema, defaultValue string) string {
	// Look up the default value in the PostgreSQL defaults mapping
	if schema != nil {
		if pgDefaults := schema.Defaults.ForProvider(types.DatabasePostgreSQL); pgDefaults != nil {
			if sqlDefault, exists := pgDefaults[defaultValue]; exists {
				return sqlDefault
			}
		}
	}

	// Check if the value is already a SQL expression (e.g. resolved by resolveFieldDefault)
	if isSQLExpression(defaultValue) {
		return defaultValue
	}

	// Numeric value
	if _, err := strconv.ParseFloat(defaultValue, 64); err == nil {
		return defaultValue
	}

	// Otherwise treat as a plain string literal
	return fmt.Sprintf("'%s'", defaultValue)
}

// GenerateAlterColumn generates one or more ALTER TABLE … ALTER COLUMN statements
// to transition a column from oldField to newField.  Each distinct change (type,
// nullability, default) becomes its own statement, separated by newlines.
// Returns an empty string when no attributes differ.
func (p *Provider) GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error) {
	var stmts []string
	tbl := p.QuoteName(tableName)
	col := p.QuoteName(newField.Name)

	// Type change
	if p.ConvertFieldType(oldField) != p.ConvertFieldType(newField) {
		stmts = append(stmts, fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s;",
			tbl, col, p.ConvertFieldType(newField)))
	}

	// Nullability change
	if oldField.IsNullable() != newField.IsNullable() {
		if newField.IsNullable() {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
				tbl, col))
		} else {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
				tbl, col))
		}
	}

	// Default change
	if oldField.Default != newField.Default {
		if newField.Default == "" {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
				tbl, col))
		} else {
			stmts = append(stmts, fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
				tbl, col, p.convertDefaultValue(nil, newField.Default)))
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

	// AutoUpdate: PostgreSQL does not support ON UPDATE natively.
	// A trigger is required to auto-update timestamp columns on row modification.

	return strings.Join(stmts, "\n"), nil
}

// GenerateForeignKeyConstraint generates an ALTER TABLE statement to add a foreign key constraint.
func (p *Provider) GenerateForeignKeyConstraint(tableName, fieldName, referencedTable, constraintName, onDelete, onUpdate string) string {
	if constraintName == "" {
		constraintName = utils.SafeConstraintName(fmt.Sprintf("fk_%s_%s", tableName, fieldName))
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

// GenerateDropForeignKeyConstraint generates an ALTER TABLE statement to drop a foreign key constraint.
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

// GetDatabaseSchema extracts schema information from a PostgreSQL database
func (p *Provider) GetDatabaseSchema(connectionString string) (*types.Schema, error) {
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	schema := &types.Schema{
		Database: types.Database{
			Name:             "extracted_schema",
			Version:          "1.0.0",
			MigrationVersion: version.GetVersion(),
		},
		Defaults: types.Defaults{
			types.DatabasePostgreSQL: {
				"blank":    "''",
				"now":      "CURRENT_TIMESTAMP",
				"new_uuid": "gen_random_uuid()",
				"today":    "CURRENT_DATE",
				"zero":     "'0'",
				"true":     "'true'",
				"false":    "'false'",
				"null":     "null",
			},
		},
		Tables: []types.Table{},
	}

	tables, err := p.extractTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tables: %w", err)
	}

	for _, table := range tables {
		fields, err := p.extractFields(db, table.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to extract fields for table %s: %w", table.Name, err)
		}
		table.Fields = fields

		indexes, err := p.extractIndexes(db, table.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to extract indexes for table %s: %w", table.Name, err)
		}
		table.Indexes = indexes

		schema.Tables = append(schema.Tables, table)
	}

	return schema, nil
}

// extractTables gets all user tables from the public schema
func (p *Provider) extractTables(db *sql.DB) ([]types.Table, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []types.Table
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}

		tables = append(tables, types.Table{
			Name:    tableName,
			Fields:  []types.Field{},
			Indexes: []types.Index{},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over table rows: %w", err)
	}

	return tables, nil
}

// extractFields gets all fields for a specific table
func (p *Provider) extractFields(db *sql.DB, tableName string) ([]types.Field, error) {
	query := `
		SELECT 
			c.column_name,
			c.data_type,
			c.character_maximum_length,
			c.numeric_precision,
			c.numeric_scale,
			c.is_nullable,
			c.column_default,
			CASE WHEN pk.column_name IS NOT NULL THEN true ELSE false END as is_primary_key
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT ku.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage ku
				ON tc.constraint_name = ku.constraint_name
			WHERE tc.table_schema = 'public'
				AND tc.table_name = $1
				AND tc.constraint_type = 'PRIMARY KEY'
		) pk ON c.column_name = pk.column_name
		WHERE c.table_schema = 'public'
			AND c.table_name = $1
		ORDER BY c.ordinal_position
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query fields for table %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()

	var fields []types.Field
	for rows.Next() {
		var (
			columnName    string
			dataType      string
			maxLength     sql.NullInt64
			numPrecision  sql.NullInt64
			numScale      sql.NullInt64
			isNullable    string
			columnDefault sql.NullString
			isPrimaryKey  bool
		)

		if err := rows.Scan(&columnName, &dataType, &maxLength, &numPrecision, &numScale, &isNullable, &columnDefault, &isPrimaryKey); err != nil {
			return nil, fmt.Errorf("failed to scan field data: %w", err)
		}

		nullable := isNullable == "YES"
		field := types.Field{
			Name:       columnName,
			Type:       p.convertSQLTypeToYAML(dataType),
			Nullable:   &nullable,
			PrimaryKey: isPrimaryKey,
		}

		// Set length, precision, scale
		if maxLength.Valid && maxLength.Int64 > 0 {
			field.Length = int(maxLength.Int64)
		}
		if numPrecision.Valid && numPrecision.Int64 > 0 {
			field.Precision = int(numPrecision.Int64)
		}
		if numScale.Valid && numScale.Int64 >= 0 {
			field.Scale = int(numScale.Int64)
		}

		// Handle default values
		if columnDefault.Valid {
			field.Default = p.convertSQLDefaultToYAML(columnDefault.String)
		}

		// Check for auto-increment/serial types
		if strings.Contains(columnDefault.String, "nextval(") {
			field.Type = "serial"
			field.Default = "" // Remove default for serial fields
		}

		fields = append(fields, field)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over field rows: %w", err)
	}

	// Get foreign key information
	fkFields, err := p.extractForeignKeys(db, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to extract foreign keys: %w", err)
	}

	// Update fields with foreign key information
	for i, field := range fields {
		if fkInfo, exists := fkFields[field.Name]; exists {
			fields[i].Type = "foreign_key"
			fields[i].ForeignKey = &types.ForeignKey{
				Table:    fkInfo.ReferencedTable,
				OnDelete: fkInfo.OnDelete,
			}
		}
	}

	return fields, nil
}

// extractForeignKeys gets foreign key constraints for a table
func (p *Provider) extractForeignKeys(db *sql.DB, tableName string) (map[string]struct {
	ReferencedTable string
	OnDelete        string
}, error) {
	query := `
		SELECT 
			kcu.column_name,
			ccu.table_name AS referenced_table,
			rc.delete_rule
		FROM information_schema.referential_constraints rc
		JOIN information_schema.key_column_usage kcu
			ON rc.constraint_name = kcu.constraint_name
		JOIN information_schema.constraint_column_usage ccu
			ON rc.unique_constraint_name = ccu.constraint_name
		WHERE kcu.table_schema = 'public'
			AND kcu.table_name = $1
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fkMap := make(map[string]struct {
		ReferencedTable string
		OnDelete        string
	})

	for rows.Next() {
		var columnName, referencedTable, deleteRule string
		if err := rows.Scan(&columnName, &referencedTable, &deleteRule); err != nil {
			return nil, fmt.Errorf("failed to scan foreign key data: %w", err)
		}

		onDelete := p.convertSQLOnDeleteToYAML(deleteRule)
		fkMap[columnName] = struct {
			ReferencedTable string
			OnDelete        string
		}{
			ReferencedTable: referencedTable,
			OnDelete:        onDelete,
		}
	}

	return fkMap, nil
}

// extractIndexes gets all indexes for a table
func (p *Provider) extractIndexes(db *sql.DB, tableName string) ([]types.Index, error) {
	query := `
		SELECT DISTINCT
			i.indexname,
			i.indexdef,
			CASE WHEN i.indexdef LIKE '%UNIQUE%' THEN true ELSE false END as is_unique
		FROM pg_indexes i
		WHERE i.schemaname = 'public'
			AND i.tablename = $1
			AND i.indexname NOT LIKE '%_pkey'  -- Exclude primary key indexes
		ORDER BY i.indexname
	`

	rows, err := db.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query indexes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var indexes []types.Index
	for rows.Next() {
		var indexName, indexDef string
		var isUnique bool

		if err := rows.Scan(&indexName, &indexDef, &isUnique); err != nil {
			return nil, fmt.Errorf("failed to scan index data: %w", err)
		}

		// Extract field names from index definition
		fields := p.parseIndexFields(indexDef)
		if len(fields) == 0 {
			continue // Skip if we can't parse fields
		}

		indexes = append(indexes, types.Index{
			Name:   indexName,
			Fields: fields,
			Unique: isUnique,
		})
	}

	return indexes, nil
}

// Helper functions for type conversion

func (p *Provider) convertSQLTypeToYAML(sqlType string) string {
	switch {
	case strings.HasPrefix(sqlType, "character varying"):
		return "varchar"
	case sqlType == "text":
		return "text"
	case sqlType == "integer":
		return "integer"
	case sqlType == "bigint":
		return "bigint"
	case sqlType == "real":
		return "float"
	case strings.HasPrefix(sqlType, "numeric"):
		return "decimal"
	case sqlType == "boolean":
		return "boolean"
	case sqlType == "date":
		return "date"
	case sqlType == "time without time zone":
		return "time"
	case strings.HasPrefix(sqlType, "timestamp"):
		return "timestamp"
	case sqlType == "uuid":
		return "uuid"
	case sqlType == "jsonb":
		return "jsonb"
	case sqlType == "json":
		return "jsonb"
	default:
		return strings.ToLower(sqlType)
	}
}

func (p *Provider) convertSQLDefaultToYAML(sqlDefault string) string {
	switch {
	case strings.Contains(sqlDefault, "CURRENT_TIMESTAMP"):
		return "now"
	case strings.Contains(sqlDefault, "CURRENT_DATE"):
		return "today"
	case strings.Contains(sqlDefault, "gen_random_uuid()"):
		return "new_uuid"
	case sqlDefault == "true":
		return "true"
	case sqlDefault == "false":
		return "false"
	case sqlDefault == "''::text" || sqlDefault == "''":
		return "blank"
	case sqlDefault == "0":
		return "zero"
	default:
		// Return the literal value, removing any type casts
		cleaned := strings.Split(sqlDefault, "::")[0]
		return strings.Trim(cleaned, "'")
	}
}

func (p *Provider) convertSQLOnDeleteToYAML(sqlOnDelete string) string {
	switch strings.ToUpper(sqlOnDelete) {
	case "CASCADE":
		return "CASCADE"
	case "RESTRICT":
		return "RESTRICT"
	case "SET NULL":
		return "SET_NULL"
	case "NO ACTION":
		return "PROTECT"
	default:
		return "PROTECT" // Default fallback
	}
}

func (p *Provider) parseIndexFields(indexDef string) []string {
	// Simple parser for index definition
	// Example: CREATE INDEX idx_name ON table_name (field1, field2)
	start := strings.Index(indexDef, "(")
	end := strings.LastIndex(indexDef, ")")

	if start == -1 || end == -1 || end <= start {
		return []string{}
	}

	fieldsStr := indexDef[start+1 : end]
	fieldsList := strings.Split(fieldsStr, ",")

	var fields []string
	for _, field := range fieldsList {
		field = strings.TrimSpace(field)
		field = strings.Trim(field, "\"") // Remove quotes
		if field != "" {
			fields = append(fields, field)
		}
	}

	return fields
}

// GenerateUpsert generates a multi-row INSERT ... ON CONFLICT DO UPDATE SET statement
// for PostgreSQL. It accepts pre-formatted SQL value literals that are inserted as-is.
// If all columns are conflict keys, DO NOTHING is used instead of DO UPDATE SET.
// Returns an empty string if valueLiterals is empty.
func (p *Provider) GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string {
	if len(valueLiterals) == 0 {
		return ""
	}

	quotedTable := p.QuoteName(table)

	quotedColumns := make([]string, len(columns))
	for i, col := range columns {
		quotedColumns[i] = p.QuoteName(col)
	}

	quotedConflictKeys := make([]string, len(conflictKeys))
	for i, key := range conflictKeys {
		quotedConflictKeys[i] = p.QuoteName(key)
	}

	// Build value rows
	rows := make([]string, len(valueLiterals))
	for i, vals := range valueLiterals {
		rows[i] = "(" + strings.Join(vals, ", ") + ")"
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(quotedTable)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(quotedColumns, ", "))
	sb.WriteString(")\nVALUES ")
	for i, row := range rows {
		if i == 0 {
			sb.WriteString(row)
		} else {
			sb.WriteString(",\n       ")
			sb.WriteString(row)
		}
	}
	sb.WriteString("\nON CONFLICT (")
	sb.WriteString(strings.Join(quotedConflictKeys, ", "))
	sb.WriteString(") ")

	// Determine non-conflict columns for the UPDATE SET clause
	conflictSet := make(map[string]struct{}, len(conflictKeys))
	for _, key := range conflictKeys {
		conflictSet[key] = struct{}{}
	}

	var updateCols []string
	for _, col := range columns {
		if _, isConflict := conflictSet[col]; !isConflict {
			updateCols = append(updateCols, col)
		}
	}

	if len(updateCols) == 0 {
		sb.WriteString("DO NOTHING;")
	} else {
		sb.WriteString("DO UPDATE SET\n")
		for i, col := range updateCols {
			quoted := p.QuoteName(col)
			sb.WriteString("  ")
			sb.WriteString(quoted)
			sb.WriteString(" = EXCLUDED.")
			sb.WriteString(quoted)
			if i < len(updateCols)-1 {
				sb.WriteString(",")
			} else {
				sb.WriteString(";")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
