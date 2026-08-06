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
package yaml

import (
	"fmt"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/ocomsoft/morphic/internal/errors"
)

// Parser handles YAML schema parsing and validation
type Parser struct {
	verbose bool
}

// NewParser creates a new YAML parser
func NewParser(verbose bool) *Parser {
	return &Parser{
		verbose: verbose,
	}
}

// ParseSchema parses a YAML schema string into a Schema struct
func (p *Parser) ParseSchema(content string) (*Schema, error) {
	if strings.TrimSpace(content) == "" {
		return nil, errors.NewValidationError("schema", "YAML content is empty")
	}

	var schema Schema
	if err := yaml.Unmarshal([]byte(content), &schema); err != nil {
		return nil, errors.NewSchemaParseError("schema.yaml", 0, fmt.Sprintf("invalid YAML syntax: %v", err))
	}

	// Validate the parsed schema
	if err := schema.Validate(); err != nil {
		return nil, errors.NewValidationError("schema", err.Error())
	}

	// Normalize field types to lowercase
	for i := range schema.Tables {
		for j := range schema.Tables[i].Fields {
			schema.Tables[i].Fields[j].Type = strings.ToLower(schema.Tables[i].Fields[j].Type)
		}
	}

	if p.verbose {
		fmt.Printf("Parsed YAML schema: %s v%s with %d tables\n",
			schema.Database.Name, schema.Database.Version, len(schema.Tables))
	}

	return &schema, nil
}

// ValidateSchemaStructure performs additional structural validation
func (p *Parser) ValidateSchemaStructure(schema *Schema) error {
	// Check for duplicate table names
	tableNames := make(map[string]bool)
	for _, table := range schema.Tables {
		if tableNames[table.Name] {
			return fmt.Errorf("duplicate table name: %s", table.Name)
		}
		tableNames[table.Name] = true

		// Check for duplicate field names within table
		fieldNames := make(map[string]bool)
		for _, field := range table.Fields {
			if fieldNames[field.Name] {
				return fmt.Errorf("table %s: duplicate field name: %s", table.Name, field.Name)
			}
			fieldNames[field.Name] = true
		}
	}

	return nil
}

// ValidateForeignKeyReferences validates that all foreign key references exist
func (p *Parser) ValidateForeignKeyReferences(schema *Schema) error {
	// Build a map of all table names for quick lookup
	tableMap := make(map[string]*Table)
	for i := range schema.Tables {
		tableMap[schema.Tables[i].Name] = &schema.Tables[i]
	}

	// Check all foreign key references
	for _, table := range schema.Tables {
		for _, field := range table.Fields {
			if field.Type == "foreign_key" && field.ForeignKey != nil {
				refTableName := field.ForeignKey.Table

				// Handle namespaced tables (e.g., "auth.User", "filesystem.FileMetaData")
				if strings.Contains(refTableName, ".") {
					// For namespaced tables, we'll just validate the format for now
					// In a full implementation, you might want to validate against known namespaces
					if p.verbose {
						fmt.Printf("Warning: Foreign key reference to namespaced table: %s\n", refTableName)
					}
					continue
				}

				// Check if the referenced table exists
				if _, exists := tableMap[refTableName]; !exists {
					return fmt.Errorf("table %s, field %s: foreign key references unknown table: %s",
						table.Name, field.Name, refTableName)
				}

				// Validate on_delete value
				validOnDelete := map[string]bool{
					"CASCADE":     true,
					"RESTRICT":    true,
					"SET_NULL":    true,
					"PROTECT":     true,
					"SET_DEFAULT": true,
					"NO_ACTION":   true,
				}
				if field.ForeignKey.OnDelete != "" && !validOnDelete[field.ForeignKey.OnDelete] {
					return fmt.Errorf("table %s, field %s: invalid on_delete value: %s",
						table.Name, field.Name, field.ForeignKey.OnDelete)
				}
			}

			if field.Type == "many_to_many" && field.ManyToMany != nil {
				refTableName := field.ManyToMany.Table

				// Handle namespaced tables
				if strings.Contains(refTableName, ".") {
					if p.verbose {
						fmt.Printf("Warning: Many-to-many reference to namespaced table: %s\n", refTableName)
					}
					continue
				}

				// Check if the referenced table exists
				if _, exists := tableMap[refTableName]; !exists {
					return fmt.Errorf("table %s, field %s: many_to_many references unknown table: %s",
						table.Name, field.Name, refTableName)
				}
			}
		}
	}

	return nil
}

// ValidateDatabaseSpecificRules validates database-specific rules
func (p *Parser) ValidateDatabaseSpecificRules(schema *Schema, databaseType DatabaseType) error {
	for _, table := range schema.Tables {
		for _, field := range table.Fields {
			if err := p.validateFieldForDatabase(&field, databaseType, table.Name); err != nil {
				return fmt.Errorf("table %s, field %s: %w", table.Name, field.Name, err)
			}
		}
	}
	return nil
}

// validateFieldForDatabase validates field-specific rules for the given database
func (p *Parser) validateFieldForDatabase(field *Field, databaseType DatabaseType, _ string) error {
	switch field.Type {
	case "varchar":
		if field.Length <= 0 {
			return fmt.Errorf("varchar field must have a positive length")
		}

		// Database-specific length limits
		switch databaseType {
		case DatabaseMySQL:
			if field.Length > 65535 {
				return fmt.Errorf("varchar length exceeds MySQL limit of 65535")
			}
		case DatabasePostgreSQL:
			if field.Length > 10485760 { // 10MB
				return fmt.Errorf("varchar length exceeds PostgreSQL limit of 10485760")
			}
		case DatabaseSQLServer:
			if field.Length > 8000 {
				return fmt.Errorf("varchar length exceeds SQL Server limit of 8000")
			}
		}

	case "decimal":
		if field.Precision <= 0 {
			return fmt.Errorf("decimal field must have a positive precision")
		}
		if field.Scale < 0 || field.Scale > field.Precision {
			return fmt.Errorf("decimal scale must be between 0 and precision (%d)", field.Precision)
		}

		// Database-specific precision limits
		switch databaseType {
		case DatabaseMySQL:
			if field.Precision > 65 {
				return fmt.Errorf("decimal precision exceeds MySQL limit of 65")
			}
		case DatabasePostgreSQL:
			if field.Precision > 1000 {
				return fmt.Errorf("decimal precision exceeds PostgreSQL limit of 1000")
			}
		case DatabaseSQLServer:
			if field.Precision > 38 {
				return fmt.Errorf("decimal precision exceeds SQL Server limit of 38")
			}
		case DatabaseSQLite:
			// SQLite doesn't have strict decimal support, warn the user
			if p.verbose {
				fmt.Printf("Warning: SQLite doesn't have native decimal support, will use NUMERIC\n")
			}
		}

	case "text":
		// SQLite and some MySQL versions have text length limits
		if field.Length > 0 {
			switch databaseType {
			case DatabaseSQLite:
				if field.Length > 1000000000 { // 1GB
					return fmt.Errorf("text length exceeds SQLite limit of 1GB")
				}
			case DatabaseMySQL:
				if field.Length > 65535 {
					return fmt.Errorf("text length exceeds MySQL TEXT limit, consider using MEDIUMTEXT or LONGTEXT")
				}
			}
		}

	case "json", "jsonb":
		// JSONB is PostgreSQL-specific
		if databaseType != DatabasePostgreSQL {
			if p.verbose {
				fmt.Printf("Warning: JSONB is PostgreSQL-specific, will be converted to appropriate JSON type for %s\n", databaseType)
			}
		}

	case "uuid":
		// UUID support varies by database
		switch databaseType {
		case DatabaseSQLite:
			if p.verbose {
				fmt.Printf("Warning: SQLite doesn't have native UUID support, will use TEXT\n")
			}
		case DatabaseMySQL:
			if p.verbose {
				fmt.Printf("Warning: MySQL doesn't have native UUID support, will use CHAR(36)\n")
			}
		}
	}

	return nil
}

// ValidateComprehensive performs all validation checks with detailed error reporting
func (p *Parser) ValidateComprehensive(schema *Schema, databaseType DatabaseType) []ValidationError {
	var errors []ValidationError

	// Basic schema validation
	if err := schema.Validate(); err != nil {
		errors = append(errors, ValidationError{
			Type:    "schema",
			Message: err.Error(),
		})
	}

	// Structural validation
	if err := p.ValidateSchemaStructure(schema); err != nil {
		errors = append(errors, ValidationError{
			Type:    "structure",
			Message: err.Error(),
		})
	}

	// Foreign key validation
	if err := p.ValidateForeignKeyReferences(schema); err != nil {
		errors = append(errors, ValidationError{
			Type:    "foreign_key",
			Message: err.Error(),
		})
	}

	// Database-specific validation
	if err := p.ValidateDatabaseSpecificRules(schema, databaseType); err != nil {
		errors = append(errors, ValidationError{
			Type:    "database_specific",
			Message: err.Error(),
		})
	}

	return errors
}

// ValidationError represents a validation error with context
type ValidationError struct {
	Type     string // "schema", "structure", "foreign_key", "database_specific"
	Table    string
	Field    string
	Message  string
	Severity string // "error", "warning"
}

// FormatValidationErrors formats validation errors for display
func (p *Parser) FormatValidationErrors(errors []ValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Schema validation failed with %d error(s):\n", len(errors))

	for i, err := range errors {
		fmt.Fprintf(&result, "  %d. [%s] %s\n", i+1, strings.ToUpper(err.Type), err.Message)
		if err.Table != "" {
			fmt.Fprintf(&result, "     Table: %s\n", err.Table)
		}
		if err.Field != "" {
			fmt.Fprintf(&result, "     Field: %s\n", err.Field)
		}
	}

	return result.String()
}

// ParseAndValidate parses and fully validates a YAML schema
func (p *Parser) ParseAndValidate(content string) (*Schema, error) {
	schema, err := p.ParseSchema(content)
	if err != nil {
		return nil, err
	}

	if err := p.ValidateSchemaStructure(schema); err != nil {
		return nil, errors.NewValidationError("schema", err.Error())
	}

	// FK validation is deferred to post-merge — individual schema files
	// may reference tables defined in sibling schema files.

	return schema, nil
}

// ParseSchemaFile parses a YAML schema file and processes includes
func (p *Parser) ParseSchemaFile(filePath string) (*Schema, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", filePath, err)
	}

	return p.ParseSchemaFileWithContent(string(content), filePath)
}

// ParseSchemaFileWithContent parses YAML content and processes includes
func (p *Parser) ParseSchemaFileWithContent(content, filePath string) (*Schema, error) {
	// First parse the base schema without processing includes
	schema, err := p.ParseSchema(content)
	if err != nil {
		return nil, err
	}

	// If there are no includes, return the schema as-is
	if len(schema.Include) == 0 {
		if p.verbose {
			fmt.Printf("No includes found in schema, returning base schema\n")
		}
		return schema, nil
	}

	// Process includes
	includeProcessor := NewIncludeProcessor(p.verbose)
	processedSchema, err := includeProcessor.ProcessIncludes(schema, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to process includes: %w", err)
	}

	// Validate structure of the merged schema (includes resolved)
	if err := p.ValidateSchemaStructure(processedSchema); err != nil {
		return nil, errors.NewValidationError("merged_schema", err.Error())
	}

	// FK validation is deferred to post-merge — this schema may reference
	// tables from sibling schema files not yet loaded.

	if p.verbose {
		fmt.Printf("Successfully processed schema with includes: %d tables total\n",
			len(processedSchema.Tables))
	}

	return processedSchema, nil
}

// GetDefaultValue gets the default value for a database type
func (p *Parser) GetDefaultValue(schema *Schema, dbType DatabaseType, defaultKey string) (string, error) {
	if !IsValidDatabase(string(dbType)) {
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}

	defaults := schema.Defaults.ForProvider(dbType)

	if defaults == nil {
		return "", fmt.Errorf("no defaults defined for database type: %s", dbType)
	}

	value, exists := defaults[defaultKey]
	if !exists {
		return "", fmt.Errorf("default value not found for key '%s' in database type '%s'", defaultKey, dbType)
	}

	return value, nil
}
