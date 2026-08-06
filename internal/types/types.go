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

// Package types defines the shared schema data structures used throughout makemigrations.
package types

import "fmt"

// Include represents an external schema to include
type Include struct {
	Module string `yaml:"module"`
	Path   string `yaml:"path"`
}

// Schema represents a YAML schema file structure
type Schema struct {
	Database     Database     `yaml:"database"`
	Include      []Include    `yaml:"include,omitempty"`
	Defaults     Defaults     `yaml:"defaults"`
	TypeMappings TypeMappings `yaml:"type_mappings"`
	Tables       []Table      `yaml:"tables"`
}

// Database represents the database metadata
type Database struct {
	Name             string `yaml:"name"`
	Version          string `yaml:"version"`
	MigrationVersion string `yaml:"migration_version,omitempty"`
}

// Defaults represents default value mappings per database type.
// It is a map from DatabaseType to a map of symbolic default keys to SQL expressions.
type Defaults map[DatabaseType]map[string]string

// UnmarshalYAML implements custom YAML unmarshaling so that YAML keys
// (lowercase strings like "postgresql") are converted to DatabaseType keys.
func (d *Defaults) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw map[string]map[string]string
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*d = make(Defaults, len(raw))
	for k, v := range raw {
		(*d)[DatabaseType(k)] = v
	}
	return nil
}

// ForProvider returns the defaults map for the given database type, or nil.
func (d Defaults) ForProvider(dbType DatabaseType) map[string]string {
	return d[dbType]
}

// SetForProvider sets the defaults map for the given database type.
// The receiver must be a non-nil map (use make(Defaults) or Defaults{} first).
func (d Defaults) SetForProvider(dbType DatabaseType, defaults map[string]string) {
	d[dbType] = defaults
}

// TypeMappings represents custom SQL type overrides per database.
// Values are SQL type strings. For parameterised types use Go template
// syntax: "DECIMAL({{.Precision}},{{.Scale}})" or "VARCHAR({{.Length}})".
// It is a map from DatabaseType to a map of abstract type names to SQL type strings.
type TypeMappings map[DatabaseType]map[string]string

// UnmarshalYAML implements custom YAML unmarshaling so that YAML keys
// (lowercase strings like "postgresql") are converted to DatabaseType keys.
func (tm *TypeMappings) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw map[string]map[string]string
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*tm = make(TypeMappings, len(raw))
	for k, v := range raw {
		(*tm)[DatabaseType(k)] = v
	}
	return nil
}

// ForProvider returns the type mapping overrides for the given database type, or nil.
func (tm TypeMappings) ForProvider(dbType DatabaseType) map[string]string {
	return tm[dbType]
}

// SetForProvider sets the type mappings for the given database type.
// The receiver must be a non-nil map (use make(TypeMappings) or TypeMappings{} first).
func (tm TypeMappings) SetForProvider(dbType DatabaseType, mappings map[string]string) {
	tm[dbType] = mappings
}

// Table represents a database table definition
type Table struct {
	Name       string  `yaml:"name"`
	Fields     []Field `yaml:"fields"`
	Indexes    []Index `yaml:"indexes,omitempty"`
	SourceFile string  `yaml:"-"`
}

// Field represents a database field/column definition
type Field struct {
	Name       string      `yaml:"name"`
	Type       string      `yaml:"type"`
	PrimaryKey bool        `yaml:"primary_key,omitempty"`
	Nullable   *bool       `yaml:"nullable,omitempty"`
	Default    string      `yaml:"default,omitempty"`
	Length     int         `yaml:"length,omitempty"`
	Precision  int         `yaml:"precision,omitempty"`
	Scale      int         `yaml:"scale,omitempty"`
	AutoCreate bool        `yaml:"auto_create,omitempty"`
	AutoUpdate bool        `yaml:"auto_update,omitempty"`
	ForeignKey *ForeignKey `yaml:"foreign_key,omitempty"`
	ManyToMany *ManyToMany `yaml:"many_to_many,omitempty"`
}

// ForeignKey represents a foreign key relationship
type ForeignKey struct {
	Table    string `yaml:"table"`
	OnDelete string `yaml:"on_delete"`
	OnUpdate string `yaml:"on_update,omitempty"`
}

// ManyToMany represents a many-to-many relationship
type ManyToMany struct {
	Table string `yaml:"table"`
}

// Index represents a database index definition
type Index struct {
	Name   string   `yaml:"name"`
	Fields []string `yaml:"fields"`
	Unique bool     `yaml:"unique,omitempty"`
	// Method specifies the index access method (e.g. BTREE, HASH, GIN, GIST, BRIN for PostgreSQL).
	// Leave empty to use the database default (usually BTREE).
	// Not supported by SQLite or SQL Server — silently ignored on those providers.
	Method string `yaml:"method,omitempty"`
	// Where is a partial index predicate expression. Only rows satisfying this
	// condition are included in the index. Leave empty for a full index.
	// Not supported by MySQL — silently ignored on that provider.
	Where string `yaml:"where,omitempty"`
	// ForeignKey is an informational annotation (e.g. "auth_user.id") indicating
	// which FK relationship this index supports. Generated by Builder tooling.
	// It does not affect SQL generation.
	ForeignKey string `yaml:"foreign_key,omitempty"`
	// FromFK marks indexes auto-generated for foreign key columns. These are
	// managed automatically by AddForeignKey/DropForeignKey and excluded from
	// schema diffs and generated migration code.
	FromFK bool `yaml:"from_fk,omitempty"`
}

// DatabaseType represents supported database types
type DatabaseType string

// DatabaseType constants enumerate the supported database backends.
const (
	DatabasePostgreSQL DatabaseType = "postgresql"
	DatabaseMySQL      DatabaseType = "mysql"
	DatabaseSQLServer  DatabaseType = "sqlserver"
	DatabaseSQLite     DatabaseType = "sqlite"
	DatabaseRedshift   DatabaseType = "redshift"
	DatabaseClickHouse DatabaseType = "clickhouse"
	DatabaseTiDB       DatabaseType = "tidb"
	DatabaseVertica    DatabaseType = "vertica"
	DatabaseYDB        DatabaseType = "ydb"
	DatabaseTurso      DatabaseType = "turso"
	DatabaseStarRocks  DatabaseType = "starrocks"
	DatabaseAuroraDSQL DatabaseType = "auroradsql"
)

// ParseDatabaseType parses a string into a DatabaseType
func ParseDatabaseType(db string) (DatabaseType, error) {
	switch DatabaseType(db) {
	case DatabasePostgreSQL, DatabaseMySQL, DatabaseSQLServer, DatabaseSQLite, DatabaseRedshift, DatabaseClickHouse, DatabaseTiDB, DatabaseVertica, DatabaseYDB, DatabaseTurso, DatabaseStarRocks, DatabaseAuroraDSQL:
		return DatabaseType(db), nil
	default:
		return "", fmt.Errorf("unsupported database type: %s (supported: postgresql, mysql, sqlserver, sqlite, redshift, clickhouse, tidb, vertica, ydb, turso, starrocks, auroradsql)", db)
	}
}

// ValidFieldTypes represents valid YAML field types
var ValidFieldTypes = map[string]bool{
	"varchar":      true,
	"text":         true,
	"integer":      true,
	"bigint":       true,
	"float":        true,
	"decimal":      true,
	"boolean":      true,
	"date":         true,
	"timestamp":    true,
	"time":         true,
	"uuid":         true,
	"json":         true,
	"jsonb":        true,
	"serial":       true,
	"bytes":        true,
	"foreign_key":  true,
	"many_to_many": true,
	"text[]":       true,
}

// IsValidFieldType checks if a field type is valid
func IsValidFieldType(fieldType string) bool {
	return ValidFieldTypes[fieldType]
}

// IsNullable returns the nullable value, defaulting to true if not set
func (f *Field) IsNullable() bool {
	if f.Nullable == nil {
		return true
	}
	return *f.Nullable
}

// SetNullable sets the nullable field
func (f *Field) SetNullable(nullable bool) {
	f.Nullable = &nullable
}

// IsValidDatabase checks if a database type is valid
func IsValidDatabase(db string) bool {
	switch DatabaseType(db) {
	case DatabasePostgreSQL, DatabaseMySQL, DatabaseSQLServer, DatabaseSQLite, DatabaseRedshift, DatabaseClickHouse, DatabaseTiDB, DatabaseVertica, DatabaseYDB, DatabaseTurso, DatabaseStarRocks, DatabaseAuroraDSQL:
		return true
	default:
		return false
	}
}

// GetTableByName finds a table by name in the schema
func (s *Schema) GetTableByName(name string) *Table {
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

// GetFieldByName finds a field by name in the table
func (t *Table) GetFieldByName(name string) *Field {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}
	return nil
}

// HasPrimaryKey checks if the table has a primary key field
func (t *Table) HasPrimaryKey() bool {
	for _, field := range t.Fields {
		if field.PrimaryKey {
			return true
		}
	}
	return false
}

// GetPrimaryKeyField returns the primary key field if it exists
func (t *Table) GetPrimaryKeyField() *Field {
	for i := range t.Fields {
		if t.Fields[i].PrimaryKey {
			return &t.Fields[i]
		}
	}
	return nil
}

// Validate validates the include structure
func (i *Include) Validate() error {
	if i.Module == "" {
		return fmt.Errorf("include module is required")
	}
	if i.Path == "" {
		return fmt.Errorf("include path is required")
	}
	return nil
}

// Validate validates the schema structure
func (s *Schema) Validate() error {
	if s.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}

	// Validate includes
	for i, include := range s.Include {
		if err := include.Validate(); err != nil {
			return fmt.Errorf("include %d: %w", i, err)
		}
	}

	if len(s.Tables) == 0 && len(s.Include) == 0 {
		return fmt.Errorf("at least one table or include is required")
	}

	for i, table := range s.Tables {
		if table.Name == "" {
			return fmt.Errorf("table %d: name is required", i)
		}

		if len(table.Fields) == 0 {
			return fmt.Errorf("table %s: at least one field is required", table.Name)
		}

		for j, field := range table.Fields {
			if err := field.Validate(); err != nil {
				return fmt.Errorf("table %s, field %d: %w", table.Name, j, err)
			}
		}

		// Validate indexes
		for j, index := range table.Indexes {
			if err := index.Validate(table); err != nil {
				return fmt.Errorf("table %s, index %d: %w", table.Name, j, err)
			}
		}
	}

	return nil
}

// Validate validates the field structure
func (f *Field) Validate() error {
	if f.Name == "" {
		return fmt.Errorf("field name is required")
	}

	if f.Type == "" {
		return fmt.Errorf("field type is required")
	}

	if !IsValidFieldType(f.Type) {
		return fmt.Errorf("invalid field type: %s", f.Type)
	}

	// Type-specific validations
	switch f.Type {
	case "varchar", "text":
		if f.Type == "varchar" && f.Length <= 0 {
			return fmt.Errorf("varchar field must have a positive length")
		}
	case "decimal":
		if f.Precision <= 0 {
			return fmt.Errorf("decimal field must have a positive precision")
		}
		if f.Scale < 0 || f.Scale > f.Precision {
			return fmt.Errorf("decimal field scale must be between 0 and precision")
		}
	case "foreign_key":
		if f.ForeignKey == nil {
			return fmt.Errorf("foreign_key field must have foreign_key definition")
		}
		if f.ForeignKey.Table == "" {
			return fmt.Errorf("foreign_key must specify a table")
		}
	case "many_to_many":
		if f.ManyToMany == nil {
			return fmt.Errorf("many_to_many field must have many_to_many definition")
		}
		if f.ManyToMany.Table == "" {
			return fmt.Errorf("many_to_many must specify a table")
		}
	}

	return nil
}

// Validate validates the index structure
func (i *Index) Validate(table Table) error {
	if i.Name == "" {
		return fmt.Errorf("index name is required")
	}

	if len(i.Fields) == 0 {
		return fmt.Errorf("index %s: at least one field is required", i.Name)
	}

	// Check that all fields in the index exist in the table
	fieldMap := make(map[string]bool)
	for _, field := range table.Fields {
		fieldMap[field.Name] = true
	}

	for _, fieldName := range i.Fields {
		if !fieldMap[fieldName] {
			return fmt.Errorf("index %s: field '%s' does not exist in table", i.Name, fieldName)
		}
	}

	return nil
}
