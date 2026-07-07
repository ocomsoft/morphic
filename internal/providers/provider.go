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
package providers

import (
	"database/sql"

	"github.com/ocomsoft/morphic/internal/types"
)

// Provider defines the interface for database-specific SQL generation
type Provider interface {
	// DDL Generation
	GenerateCreateTable(schema *types.Schema, table *types.Table) (string, error)
	GenerateDropTable(tableName string) string
	// GenerateDropTableCascade drops a table and any dependent objects (e.g. FK constraints).
	// Used during rollback so that FK ordering in the migration does not cause failures.
	// Providers that do not support CASCADE should return the same as GenerateDropTable.
	GenerateDropTableCascade(tableName string) string
	GenerateAddColumn(tableName string, field *types.Field) string
	GenerateDropColumn(tableName, columnName string) string
	GenerateAlterColumn(tableName string, oldField, newField *types.Field) (string, error)
	GenerateRenameTable(oldName, newName string) string
	GenerateRenameColumn(tableName, oldName, newName string) string

	// Index Operations
	GenerateCreateIndex(index *types.Index, tableName string) string
	GenerateDropIndex(indexName, tableName string) string

	// Foreign Key Operations
	GenerateForeignKeyConstraint(tableName, fieldName, referencedTable, constraintName, onDelete, onUpdate string) string
	GenerateDropForeignKeyConstraint(tableName, constraintName string) string
	GenerateJunctionTable(table1, table2 string, schema *types.Schema) (string, error)
	InferForeignKeyType(referencedTable string, schema *types.Schema) string

	// Type Conversion
	ConvertFieldType(field *types.Field) string
	GetDefaultValue(defaultRef string, defaults map[string]string) (string, error)

	// Utilities
	QuoteName(name string) string
	SupportsOperation(operation string) bool
	// Placeholder returns the bind-parameter placeholder for the nth argument (1-indexed).
	// Examples: PostgreSQL uses $1, SQL Server uses @p1, most others use ?.
	Placeholder(n int) string
	// HistoryTableDDL returns the CREATE TABLE IF NOT EXISTS statement for the
	// morphic_history migration-tracking table using this provider's SQL dialect.
	HistoryTableDDL() string
	// IsNotFoundError returns true when err indicates that a DROP operation
	// targeted an object that does not exist in the database.
	// Used by the runner to warn-and-continue rather than fail.
	IsNotFoundError(err error) bool
	// IsAlreadyExistsError returns true when err indicates that a CREATE operation
	// targeted an object that already exists in the database.
	// Used by the runner during rollback to skip re-creating objects that are already present
	// (e.g. after a partial rollback left the database in an intermediate state).
	IsAlreadyExistsError(err error) bool
	// TableColumns returns the column names for the given table in the database.
	// Returns nil, nil if the table does not exist (not an error).
	// Returns nil, err on query failure or if this provider does not support introspection.
	TableColumns(db *sql.DB, tableName string) ([]string, error)

	// GenerateUpsert generates a database-appropriate upsert statement for one or more
	// rows. columns is the ordered list of column names; valueLiterals is a parallel
	// slice of pre-formatted SQL literal strings (e.g. "'AU'", "42", "NULL") for each
	// row. conflictKeys identifies which columns to check for conflict or match on.
	//
	// Implementations should use the provider's quoting convention (QuoteName) and the
	// appropriate dialect:
	//   - PostgreSQL/AuroraDSQL: INSERT … ON CONFLICT (keys) DO UPDATE SET col = EXCLUDED.col
	//   - MySQL/TiDB/StarRocks:  INSERT … ON DUPLICATE KEY UPDATE col = VALUES(col)
	//   - SQLite/Turso:          INSERT … ON CONFLICT(keys) DO UPDATE SET col = excluded.col
	//   - SQL Server/Vertica:    MERGE INTO … USING … ON … WHEN MATCHED … WHEN NOT MATCHED …
	//   - Redshift:              DELETE … + INSERT (no native ON CONFLICT)
	//   - ClickHouse:            INSERT (deduplication via ReplacingMergeTree)
	//   - YDB:                   UPSERT INTO …
	GenerateUpsert(table string, conflictKeys []string, columns []string, valueLiterals [][]string) string

	// Schema processing
	GenerateIndexes(schema *types.Schema) string
	GenerateForeignKeyConstraints(schema *types.Schema, junctionTables []types.Table) string

	// Database reverse engineering
	GetDatabaseSchema(connectionString string) (*types.Schema, error)

	// Configuration
	SetTypeMappings(mappings map[string]string)
}

// TableRecreationProvider is an optional interface implemented by providers
// (such as SQLite) that require the full current table definition to perform
// column alterations. SQLite does not support ALTER COLUMN natively, so it
// must recreate the table: create a temp table with the new definition, copy
// all rows, drop the old table, and rename the temp table.
//
// When a provider implements this interface, AlterField.Up/Down uses it
// instead of GenerateAlterColumn, passing the complete current table so the
// provider has all column definitions and indexes needed for recreation.
//
// fromField is the column's current definition in the database.
// toField is the desired target definition after the operation.
type TableRecreationProvider interface {
	GenerateAlterColumnWithTable(currentTable *types.Table, fromField, toField *types.Field) (string, error)
}
