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

// operations.go defines the Operation interface and all concrete migration
// operation types used by the morphic Go migration framework.
package migrate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ocomsoft/morphic/internal/providers"
	"github.com/ocomsoft/morphic/internal/types"
)

// Operation is the interface all migration operations must implement.
// Each operation can generate forward (Up) and reverse (Down) SQL, mutate the
// in-memory SchemaState for graph traversal, and describe itself for display.
// Up/Down method names align with the CLI commands `./migrate up` / `./migrate down`.
type Operation interface {
	// Up generates the forward SQL for this operation.
	Up(p providers.Provider, state *SchemaState, defaults map[string]string, migrationsDir string) (string, error)
	// Down generates the reverse SQL to undo this operation.
	Down(p providers.Provider, state *SchemaState, defaults map[string]string, migrationsDir string) (string, error)
	// Mutate applies this operation to the given SchemaState, updating the
	// in-memory schema representation during migration graph traversal.
	Mutate(state *SchemaState) error
	// Describe returns a human-readable description of the operation.
	Describe() string
	// TypeName returns the operation type identifier (e.g. "create_table").
	TypeName() string
	// TableName returns the primary table this operation acts on, or "" for RunSQL.
	TableName() string
	// IsDestructive returns true if this operation may cause data loss.
	IsDestructive() bool
}

// ErrorIgnorer is an optional interface implemented by operations that support
// ignoring execution errors at the runner level. When ShouldIgnoreErrors
// returns true, the runner logs a warning and continues instead of aborting.
type ErrorIgnorer interface {
	ShouldIgnoreErrors() bool
}

// boolPtr converts a bool value to a *bool pointer for use with types.Field.Nullable.
func boolPtr(b bool) *bool { return &b }

// resolveFieldDefault resolves a symbolic default key (e.g. "uuid") to its
// SQL expression (e.g. "uuid_generate_v4()") using the active defaults map.
// The field is modified in place; if no mapping exists the value is unchanged.
func resolveFieldDefault(f *types.Field, defaults map[string]string) {
	if f.Default != "" && len(defaults) > 0 {
		if resolved, ok := defaults[f.Default]; ok {
			f.Default = resolved
		}
	}
}

// toTypesField converts a migrate.Field (with bool Nullable) to a *types.Field
// (with *bool Nullable) as required by the providers.Provider interface.
func toTypesField(f Field) *types.Field {
	tf := &types.Field{
		Name:       f.Name,
		Type:       f.Type,
		PrimaryKey: f.PrimaryKey,
		Nullable:   boolPtr(f.Nullable),
		Default:    f.Default,
		Length:     f.Length,
		Precision:  f.Precision,
		Scale:      f.Scale,
		AutoCreate: f.AutoCreate,
		AutoUpdate: f.AutoUpdate,
	}
	if f.ForeignKey != nil {
		tf.ForeignKey = &types.ForeignKey{
			Table:    f.ForeignKey.Table,
			OnDelete: f.ForeignKey.OnDelete,
			OnUpdate: f.ForeignKey.OnUpdate,
		}
	}
	if f.ManyToMany != nil {
		tf.ManyToMany = &types.ManyToMany{Table: f.ManyToMany.Table}
	}
	return tf
}

// stateToSchema builds a minimal types.Schema from a SchemaState for provider
// calls that need the full schema (e.g. GenerateCreateTable for FK resolution).
func stateToSchema(state *SchemaState) *types.Schema {
	if state == nil {
		return &types.Schema{}
	}
	s := &types.Schema{}
	for _, ts := range state.Tables {
		t := &types.Table{Name: ts.Name}
		for _, f := range ts.Fields {
			t.Fields = append(t.Fields, *toTypesField(f))
		}
		for _, idx := range ts.Indexes {
			t.Indexes = append(t.Indexes, types.Index{
				Name:   idx.Name,
				Fields: idx.Fields,
				Unique: idx.Unique,
				Method: idx.Method,
				Where:  idx.Where,
				FromFK: idx.FromFK,
			})
		}
		s.Tables = append(s.Tables, *t)
	}
	return s
}

// joinFields joins a slice of field names into a comma-separated string.
func joinFields(fields []string) string {
	return strings.Join(fields, ", ")
}

// --- CreateTable ---

// CreateTable is a migration operation that creates a new database table
// with the specified fields and indexes.
// When SchemaOnly is true the operation advances the in-memory schema state
// (via Mutate) but does not execute any SQL, allowing the schema state to be
// seeded from an existing database without re-running CREATE TABLE.
type CreateTable struct {
	Name         string
	Fields       []Field
	Indexes      []Index
	SchemaOnly   bool // when true, Up/Down return no SQL; Mutate still runs
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *CreateTable) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *CreateTable) TypeName() string { return "create_table" }

// TableName returns the name of the table being created.
func (op *CreateTable) TableName() string { return op.Name }

// IsDestructive returns false — creating a table is not destructive.
func (op *CreateTable) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *CreateTable) Describe() string {
	return fmt.Sprintf("Create table %s (%d fields)", op.Name, len(op.Fields))
}

// Up generates the CREATE TABLE SQL statement, or returns empty string when SchemaOnly is set.
// Field defaults are resolved against the active defaults map before being passed to the provider.
func (op *CreateTable) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	schema := stateToSchema(state)
	table := &types.Table{Name: op.Name}
	for _, f := range op.Fields {
		tf := toTypesField(f)
		resolveFieldDefault(tf, defaults)
		table.Fields = append(table.Fields, *tf)
	}
	for _, idx := range op.Indexes {
		table.Indexes = append(table.Indexes, types.Index{Name: idx.Name, Fields: idx.Fields, Unique: idx.Unique, Method: idx.Method, Where: idx.Where})
	}
	return p.GenerateCreateTable(schema, table)
}

// Down generates the DROP TABLE CASCADE SQL to reverse the creation.
// Uses GenerateDropTableCascade so that any dependent objects (e.g. foreign key
// constraints from other tables) are automatically removed, preventing ordering
// failures when multiple CreateTable operations are rolled back together.
// Returns empty string when SchemaOnly is set.
func (op *CreateTable) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return p.GenerateDropTableCascade(op.Name), nil
}

// Mutate adds the new table to the SchemaState.
func (op *CreateTable) Mutate(state *SchemaState) error {
	return state.AddTable(op.Name, op.Fields, op.Indexes)
}

// --- DropTable ---

// DropTable is a migration operation that drops an existing database table.
// When SchemaOnly is true the operation advances the in-memory schema state
// (via Mutate) but does not execute any SQL against the database, allowing the
// developer to acknowledge a removal in the schema definition without
// immediately dropping the live table.
type DropTable struct {
	Name         string
	SchemaOnly   bool // when true, Up/Down return no SQL; Mutate still runs
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *DropTable) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *DropTable) TypeName() string { return "drop_table" }

// TableName returns the name of the table being dropped.
func (op *DropTable) TableName() string { return op.Name }

// IsDestructive returns true — dropping a table causes data loss.
func (op *DropTable) IsDestructive() bool { return true }

// Describe returns a human-readable description of this operation.
func (op *DropTable) Describe() string { return fmt.Sprintf("Drop table %s", op.Name) }

// Up generates the DROP TABLE SQL statement, or returns empty string when SchemaOnly is set.
func (op *DropTable) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return p.GenerateDropTable(op.Name), nil
}

// Down reconstructs the CREATE TABLE SQL by reading the table's pre-drop state.
// Returns empty string when SchemaOnly is set.
func (op *DropTable) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	ts, exists := state.Tables[op.Name]
	if !exists {
		return "", fmt.Errorf("table %q not found in state for Down generation", op.Name)
	}
	schema := stateToSchema(state)
	t := &types.Table{Name: ts.Name}
	for _, f := range ts.Fields {
		tf := toTypesField(f)
		resolveFieldDefault(tf, defaults)
		t.Fields = append(t.Fields, *tf)
	}
	for _, idx := range ts.Indexes {
		t.Indexes = append(t.Indexes, types.Index{Name: idx.Name, Fields: idx.Fields, Unique: idx.Unique, Method: idx.Method, Where: idx.Where})
	}
	return p.GenerateCreateTable(schema, t)
}

// Mutate removes the table from the SchemaState.
func (op *DropTable) Mutate(state *SchemaState) error { return state.DropTable(op.Name) }

// --- RenameTable ---

// RenameTable is a migration operation that renames an existing database table.
type RenameTable struct{ OldName, NewName string }

// TypeName returns the operation type identifier.
func (op *RenameTable) TypeName() string { return "rename_table" }

// TableName returns the old (pre-rename) table name.
func (op *RenameTable) TableName() string { return op.OldName }

// IsDestructive returns false — renaming a table is not destructive.
func (op *RenameTable) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *RenameTable) Describe() string {
	return fmt.Sprintf("Rename table %s to %s", op.OldName, op.NewName)
}

// Up generates the RENAME TABLE SQL statement.
func (op *RenameTable) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateRenameTable(op.OldName, op.NewName), nil
}

// Down generates the reverse RENAME TABLE SQL to restore the original name.
func (op *RenameTable) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateRenameTable(op.NewName, op.OldName), nil
}

// Mutate updates the SchemaState to reflect the renamed table.
func (op *RenameTable) Mutate(state *SchemaState) error {
	return state.RenameTable(op.OldName, op.NewName)
}

// --- AddField ---

// AddField is a migration operation that adds a new column to an existing table.
// When SchemaOnly is true the operation advances the in-memory schema state
// (via Mutate) but does not execute any SQL, allowing the schema state to be
// seeded from an existing database without running ALTER TABLE ADD COLUMN.
type AddField struct {
	Table        string
	Field        Field
	SchemaOnly   bool // when true, Up/Down return no SQL; Mutate still runs
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *AddField) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *AddField) TypeName() string { return "add_field" }

// TableName returns the name of the table being altered.
func (op *AddField) TableName() string { return op.Table }

// IsDestructive returns false — adding a column is not destructive.
func (op *AddField) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *AddField) Describe() string {
	return fmt.Sprintf("Add field %s.%s %s", op.Table, op.Field.Name, op.Field.Type)
}

// Up generates the ADD COLUMN SQL statement, or returns empty string when SchemaOnly is set.
// The field default is resolved against the active defaults map before being passed to the provider.
func (op *AddField) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	tf := toTypesField(op.Field)
	resolveFieldDefault(tf, defaults)
	return p.GenerateAddColumn(op.Table, tf), nil
}

// Down generates the DROP COLUMN SQL to reverse the addition.
// Returns empty string when SchemaOnly is set.
func (op *AddField) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return p.GenerateDropColumn(op.Table, op.Field.Name), nil
}

// Mutate adds the new field to the table's entry in SchemaState.
func (op *AddField) Mutate(state *SchemaState) error {
	return state.AddField(op.Table, op.Field)
}

// --- DropField ---

// DropField is a migration operation that removes a column from an existing table.
// When SchemaOnly is true the operation advances the in-memory schema state
// (via Mutate) but does not execute any SQL against the database, allowing the
// developer to acknowledge a removal in the schema definition without
// immediately dropping the live column.
type DropField struct {
	Table        string
	Field        string
	SchemaOnly   bool // when true, Up/Down return no SQL; Mutate still runs
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *DropField) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *DropField) TypeName() string { return "drop_field" }

// TableName returns the name of the table being altered.
func (op *DropField) TableName() string { return op.Table }

// IsDestructive returns true — dropping a column causes data loss.
func (op *DropField) IsDestructive() bool { return true }

// Describe returns a human-readable description of this operation.
func (op *DropField) Describe() string {
	return fmt.Sprintf("Drop field %s.%s", op.Table, op.Field)
}

// Up generates the DROP COLUMN SQL statement, or returns empty string when SchemaOnly is set.
func (op *DropField) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return p.GenerateDropColumn(op.Table, op.Field), nil
}

// Down reconstructs the ADD COLUMN SQL by reading the field's pre-drop state.
// Returns empty string when SchemaOnly is set.
// The field default is resolved against the active defaults map before being passed to the provider.
func (op *DropField) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	ts, exists := state.Tables[op.Table]
	if !exists {
		return "", fmt.Errorf("table %q not found in state", op.Table)
	}
	for _, f := range ts.Fields {
		if f.Name == op.Field {
			tf := toTypesField(f)
			resolveFieldDefault(tf, defaults)
			return p.GenerateAddColumn(op.Table, tf), nil
		}
	}
	return "", fmt.Errorf("field %q not found in table %q state", op.Field, op.Table)
}

// Mutate removes the field from the table's entry in SchemaState.
func (op *DropField) Mutate(state *SchemaState) error {
	return state.DropField(op.Table, op.Field)
}

// --- AlterField ---

// AlterField is a migration operation that modifies an existing column's definition.
type AlterField struct {
	Table    string
	OldField Field
	NewField Field
}

// TypeName returns the operation type identifier.
func (op *AlterField) TypeName() string { return "alter_field" }

// TableName returns the name of the table being altered.
func (op *AlterField) TableName() string { return op.Table }

// IsDestructive returns false — altering a column is not considered destructive
// (though data conversion may fail at the database level for incompatible types).
func (op *AlterField) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *AlterField) Describe() string {
	return fmt.Sprintf("Alter field %s.%s", op.Table, op.NewField.Name)
}

// tableStateToTypesTable converts a table entry from SchemaState to *types.Table
// so that providers implementing TableRecreationProvider can access the full
// current table definition when performing column alterations that require
// recreating the table (e.g. SQLite).
func tableStateToTypesTable(state *SchemaState, tableName string, defaults map[string]string) *types.Table {
	t := &types.Table{Name: tableName}
	if state == nil {
		return t
	}
	ts, ok := state.Tables[tableName]
	if !ok {
		return t
	}
	for _, f := range ts.Fields {
		tf := toTypesField(f)
		resolveFieldDefault(tf, defaults)
		t.Fields = append(t.Fields, *tf)
	}
	for _, idx := range ts.Indexes {
		t.Indexes = append(t.Indexes, types.Index{Name: idx.Name, Fields: idx.Fields, Unique: idx.Unique, Method: idx.Method, Where: idx.Where})
	}
	return t
}

// Up generates the ALTER COLUMN SQL to apply the new field definition.
// If the provider implements TableRecreationProvider (e.g. SQLite), the full
// current table definition is passed so the provider can recreate the table.
// Field defaults are resolved against the active defaults map before use.
func (op *AlterField) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	oldF := toTypesField(op.OldField)
	newF := toTypesField(op.NewField)
	resolveFieldDefault(oldF, defaults)
	resolveFieldDefault(newF, defaults)
	if trp, ok := p.(providers.TableRecreationProvider); ok {
		t := tableStateToTypesTable(state, op.Table, defaults)
		return trp.GenerateAlterColumnWithTable(t, oldF, newF)
	}
	return p.GenerateAlterColumn(op.Table, oldF, newF)
}

// Down generates the ALTER COLUMN SQL to restore the original field definition.
// If the provider implements TableRecreationProvider (e.g. SQLite), the full
// current table definition is passed so the provider can recreate the table.
// Field defaults are resolved against the active defaults map before use.
func (op *AlterField) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	oldF := toTypesField(op.OldField)
	newF := toTypesField(op.NewField)
	resolveFieldDefault(oldF, defaults)
	resolveFieldDefault(newF, defaults)
	if trp, ok := p.(providers.TableRecreationProvider); ok {
		// For Down, state reflects the post-Up schema; pass newF→oldF to recreate to original.
		t := tableStateToTypesTable(state, op.Table, defaults)
		return trp.GenerateAlterColumnWithTable(t, newF, oldF)
	}
	return p.GenerateAlterColumn(op.Table, newF, oldF)
}

// Mutate replaces the field in the table's entry in SchemaState.
func (op *AlterField) Mutate(state *SchemaState) error {
	return state.AlterField(op.Table, op.NewField)
}

// --- RenameField ---

// RenameField is a migration operation that renames a column in an existing table.
type RenameField struct {
	Table   string
	OldName string
	NewName string
}

// TypeName returns the operation type identifier.
func (op *RenameField) TypeName() string { return "rename_field" }

// TableName returns the name of the table being altered.
func (op *RenameField) TableName() string { return op.Table }

// IsDestructive returns false — renaming a column is not destructive.
func (op *RenameField) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *RenameField) Describe() string {
	return fmt.Sprintf("Rename field %s.%s to %s", op.Table, op.OldName, op.NewName)
}

// Up generates the RENAME COLUMN SQL statement.
func (op *RenameField) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateRenameColumn(op.Table, op.OldName, op.NewName), nil
}

// Down generates the reverse RENAME COLUMN SQL to restore the original name.
func (op *RenameField) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateRenameColumn(op.Table, op.NewName, op.OldName), nil
}

// Mutate updates the field name in the table's entry in SchemaState.
func (op *RenameField) Mutate(state *SchemaState) error {
	return state.RenameField(op.Table, op.OldName, op.NewName)
}

// --- AddIndex ---

// AddIndex is a migration operation that adds an index to an existing table.
type AddIndex struct {
	Table        string
	Index        Index
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *AddIndex) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *AddIndex) TypeName() string { return "add_index" }

// TableName returns the name of the table being indexed.
func (op *AddIndex) TableName() string { return op.Table }

// IsDestructive returns false — adding an index is not destructive.
func (op *AddIndex) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *AddIndex) Describe() string {
	return fmt.Sprintf("Add index %s on %s(%s)", op.Index.Name, op.Table, joinFields(op.Index.Fields))
}

// Up generates the CREATE INDEX SQL statement.
func (op *AddIndex) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	ti := &types.Index{Name: op.Index.Name, Unique: op.Index.Unique, Fields: op.Index.Fields, Method: op.Index.Method, Where: op.Index.Where}
	return p.GenerateCreateIndex(ti, op.Table), nil
}

// Down generates the DROP INDEX SQL to reverse the index creation.
func (op *AddIndex) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateDropIndex(op.Index.Name, op.Table), nil
}

// Mutate adds the index to the table's entry in SchemaState.
func (op *AddIndex) Mutate(state *SchemaState) error {
	return state.AddIndex(op.Table, op.Index)
}

// --- DropIndex ---

// DropIndex is a migration operation that removes an index from an existing table.
type DropIndex struct {
	Table        string
	Index        string
	IgnoreErrors bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *DropIndex) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *DropIndex) TypeName() string { return "drop_index" }

// TableName returns the name of the table whose index is being dropped.
func (op *DropIndex) TableName() string { return op.Table }

// IsDestructive returns false — dropping an index is not considered destructive.
func (op *DropIndex) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *DropIndex) Describe() string {
	return fmt.Sprintf("Drop index %s on %s", op.Index, op.Table)
}

// Up generates the DROP INDEX SQL statement.
func (op *DropIndex) Up(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	return p.GenerateDropIndex(op.Index, op.Table), nil
}

// Down reconstructs the CREATE INDEX SQL by reading the index's pre-drop state.
func (op *DropIndex) Down(p providers.Provider, state *SchemaState, defaults map[string]string, _ string) (string, error) {
	ts, exists := state.Tables[op.Table]
	if !exists {
		return "", fmt.Errorf("table %q not found in state", op.Table)
	}
	for _, idx := range ts.Indexes {
		if idx.Name == op.Index {
			ti := &types.Index{Name: idx.Name, Unique: idx.Unique, Fields: idx.Fields, Method: idx.Method, Where: idx.Where}
			return p.GenerateCreateIndex(ti, op.Table), nil
		}
	}
	return "", fmt.Errorf("index %q not found in table %q state", op.Index, op.Table)
}

// Mutate removes the index from the table's entry in SchemaState.
func (op *DropIndex) Mutate(state *SchemaState) error {
	return state.DropIndex(op.Table, op.Index)
}

// normalizeOnDelete converts Django-style on_delete values to their SQL
// equivalents. SQL keywords are returned unchanged.
func normalizeOnDelete(v string) string {
	switch strings.ToUpper(v) {
	case "PROTECT":
		return "RESTRICT"
	case "SET_NULL":
		return "SET NULL"
	case "SET_DEFAULT":
		return "SET DEFAULT"
	case "DO_NOTHING":
		return "NO ACTION"
	default:
		return v
	}
}

// --- AddForeignKey ---

// AddForeignKey is a migration operation that adds a foreign key constraint to
// an existing table using ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY.
// The FK column must already exist (created by AddField or CreateTable).
type AddForeignKey struct {
	Table           string
	FieldName       string
	ConstraintName  string
	ReferencedTable string
	OnDelete        string
	OnUpdate        string
	IgnoreErrors    bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *AddForeignKey) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *AddForeignKey) TypeName() string { return "add_foreign_key" }

// TableName returns the name of the table the constraint is added to.
func (op *AddForeignKey) TableName() string { return op.Table }

// IsDestructive returns false — adding a FK constraint is not destructive.
func (op *AddForeignKey) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *AddForeignKey) Describe() string {
	return fmt.Sprintf("Add foreign key %s on %s.%s → %s", op.ConstraintName, op.Table, op.FieldName, op.ReferencedTable)
}

// Up generates the ALTER TABLE ... ADD CONSTRAINT ... FOREIGN KEY SQL.
// Django-style on_delete values are normalised to their SQL equivalents:
//   - PROTECT     → RESTRICT  (Django raises ProtectedError; SQL enforces at DB level)
//   - SET_NULL    → SET NULL
//   - SET_DEFAULT → SET DEFAULT
//   - DO_NOTHING  → NO ACTION
func (op *AddForeignKey) Up(p providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	onDelete := normalizeOnDelete(op.OnDelete)
	onUpdate := normalizeOnDelete(op.OnUpdate)
	return p.GenerateForeignKeyConstraint(op.Table, op.FieldName, op.ReferencedTable, op.ConstraintName, onDelete, onUpdate), nil
}

// Down generates the ALTER TABLE ... DROP CONSTRAINT SQL to remove the FK.
func (op *AddForeignKey) Down(p providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return p.GenerateDropForeignKeyConstraint(op.Table, op.ConstraintName), nil
}

// Mutate records the foreign key in the SchemaState.
func (op *AddForeignKey) Mutate(state *SchemaState) error {
	return state.AddForeignKey(op.Table, ForeignKeyConstraint{
		Name:            op.ConstraintName,
		FieldName:       op.FieldName,
		ReferencedTable: op.ReferencedTable,
		OnDelete:        op.OnDelete,
		OnUpdate:        op.OnUpdate,
	})
}

// --- DropForeignKey ---

// DropForeignKey is a migration operation that drops a foreign key constraint
// from an existing table using ALTER TABLE ... DROP CONSTRAINT.
// The Down method reads the pre-drop FK definition from SchemaState to
// reconstruct the ADD CONSTRAINT SQL — state must still contain the FK when
// Down is called (i.e. before Mutate runs, which is the runner's guarantee).
type DropForeignKey struct {
	Table          string
	ConstraintName string
	IgnoreErrors   bool // when true, runner logs a warning and continues on SQL failure
}

// ShouldIgnoreErrors implements ErrorIgnorer.
func (op *DropForeignKey) ShouldIgnoreErrors() bool { return op.IgnoreErrors }

// TypeName returns the operation type identifier.
func (op *DropForeignKey) TypeName() string { return "drop_foreign_key" }

// TableName returns the name of the table the constraint is removed from.
func (op *DropForeignKey) TableName() string { return op.Table }

// IsDestructive returns true — removing a FK constraint changes referential integrity.
func (op *DropForeignKey) IsDestructive() bool { return true }

// Describe returns a human-readable description of this operation.
func (op *DropForeignKey) Describe() string {
	return fmt.Sprintf("Drop foreign key %s from %s", op.ConstraintName, op.Table)
}

// Up generates the ALTER TABLE ... DROP CONSTRAINT SQL.
func (op *DropForeignKey) Up(p providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return p.GenerateDropForeignKeyConstraint(op.Table, op.ConstraintName), nil
}

// Down reconstructs the ADD CONSTRAINT SQL by reading the FK's pre-drop state.
func (op *DropForeignKey) Down(p providers.Provider, state *SchemaState, _ map[string]string, _ string) (string, error) {
	ts, exists := state.Tables[op.Table]
	if !exists {
		return "", fmt.Errorf("table %q not found in state", op.Table)
	}
	for _, fk := range ts.ForeignKeys {
		if fk.Name == op.ConstraintName {
			return p.GenerateForeignKeyConstraint(op.Table, fk.FieldName, fk.ReferencedTable, fk.Name, fk.OnDelete, fk.OnUpdate), nil
		}
	}
	return "", fmt.Errorf("foreign key %q not found in table %q state", op.ConstraintName, op.Table)
}

// Mutate removes the foreign key from the SchemaState.
func (op *DropForeignKey) Mutate(state *SchemaState) error {
	return state.DropForeignKey(op.Table, op.ConstraintName)
}

// --- RunSQL ---

// RunSQL is a migration operation that executes raw SQL for forward and reverse
// migrations. ForwardSQL and BackwardSQL field names are used (not Forward/Backward)
// to avoid conflict with the Up/Down method names on the Operation interface.
// When SchemaOnly is true the operation is recorded as applied but no SQL is sent
// to the database — useful when the schema is already in place.
type RunSQL struct {
	// ForwardSQL is the raw SQL to execute on Up (forward migration).
	ForwardSQL string
	// BackwardSQL is the raw SQL to execute on Down (reverse migration).
	BackwardSQL string
	// SchemaOnly marks the operation as applied without executing any SQL.
	// Use this when the underlying schema already exists in the database.
	SchemaOnly bool
}

// TypeName returns the operation type identifier.
func (op *RunSQL) TypeName() string { return "run_sql" }

// TableName returns an empty string — RunSQL does not target a specific table.
func (op *RunSQL) TableName() string { return "" }

// IsDestructive returns false — RunSQL destructiveness depends on the SQL content.
func (op *RunSQL) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
// Includes a truncated preview of the SQL to aid debugging.
func (op *RunSQL) Describe() string {
	sql := op.ForwardSQL
	if len(sql) > 80 {
		sql = sql[:80] + "…"
	}
	return fmt.Sprintf("Run SQL: %s", sql)
}

// Up returns the ForwardSQL string, or empty string when SchemaOnly is set.
func (op *RunSQL) Up(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return op.ForwardSQL, nil
}

// Down returns the BackwardSQL string, or empty string when SchemaOnly is set.
func (op *RunSQL) Down(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	if op.SchemaOnly {
		return "", nil
	}
	return op.BackwardSQL, nil
}

// Mutate is a no-op for RunSQL — raw SQL does not alter the SchemaState.
func (op *RunSQL) Mutate(_ *SchemaState) error { return nil }

// --- SetDefaults ---

// SetDefaults is a migration operation that records the active schema defaults
// (e.g. {"uuid": "uuid_generate_v4()"}) into the SchemaState so that subsequent
// operations can resolve symbolic default references at runtime.
// It emits no SQL — its sole purpose is to update the in-memory state.
type SetDefaults struct {
	Defaults map[string]string
}

// TypeName returns the operation type identifier.
func (op *SetDefaults) TypeName() string { return "set_defaults" }

// TableName returns "" — SetDefaults does not target a specific table.
func (op *SetDefaults) TableName() string { return "" }

// IsDestructive returns false — setting defaults is not destructive.
func (op *SetDefaults) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *SetDefaults) Describe() string { return "Set schema defaults" }

// Up returns empty string — SetDefaults emits no SQL.
func (op *SetDefaults) Up(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return "", nil
}

// Down returns empty string — SetDefaults emits no SQL.
func (op *SetDefaults) Down(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return "", nil
}

// Mutate updates the active schema defaults on the SchemaState.
func (op *SetDefaults) Mutate(state *SchemaState) error {
	state.SetDefaults(op.Defaults)
	return nil
}

// --- SetTypeMappings ---

// SetTypeMappings is a migration operation that records the active schema type mappings
// for the target database provider. It emits no SQL — its only effect is updating
// SchemaState.TypeMappings so that subsequent operations and the runner use the correct
// type overrides when generating SQL via ConvertFieldType.
type SetTypeMappings struct {
	TypeMappings map[string]string
}

// TypeName returns the operation type identifier.
func (op *SetTypeMappings) TypeName() string { return "set_type_mappings" }

// TableName returns "" — SetTypeMappings does not target a specific table.
func (op *SetTypeMappings) TableName() string { return "" }

// IsDestructive returns false — SetTypeMappings is always non-destructive.
func (op *SetTypeMappings) IsDestructive() bool { return false }

// Describe returns a human-readable description.
func (op *SetTypeMappings) Describe() string { return "Set schema type mappings" }

// Up returns empty string — SetTypeMappings emits no SQL.
func (op *SetTypeMappings) Up(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return "", nil
}

// Down returns empty string — SetTypeMappings emits no SQL.
func (op *SetTypeMappings) Down(_ providers.Provider, _ *SchemaState, _ map[string]string, _ string) (string, error) {
	return "", nil
}

// Mutate applies the type mappings to the schema state.
func (op *SetTypeMappings) Mutate(state *SchemaState) error {
	state.SetTypeMappings(op.TypeMappings)
	return nil
}

// --- UpsertData ---

// UpsertData is a migration operation that inserts or updates rows in a table.
// It is designed for seeding reference data (e.g. country codes, status enums,
// configuration rows) as part of a migration.
//
// The operation generates database-appropriate upsert SQL via the active provider:
//   - PostgreSQL/AuroraDSQL: INSERT … ON CONFLICT … DO UPDATE SET
//   - MySQL/TiDB/StarRocks:  INSERT … ON DUPLICATE KEY UPDATE
//   - SQLite/Turso:          INSERT … ON CONFLICT … DO UPDATE SET
//   - SQL Server/Vertica:    MERGE INTO … USING … WHEN MATCHED / NOT MATCHED
//   - Redshift:              DELETE … ; INSERT …
//   - ClickHouse:            INSERT (dedup via ReplacingMergeTree)
//   - YDB:                   UPSERT INTO …
//
// Column order in the generated SQL is determined by sorting the keys of the
// first row alphabetically, so all rows must contain the same set of keys.
//
// Rollback (Down) deletes each row by matching on ConflictKeys.
type UpsertData struct {
	// Table is the target table name.
	Table string
	// ConflictKeys lists the column names used to detect conflicts (primary key
	// or unique constraint columns). Used in ON CONFLICT / MERGE ON / DELETE WHERE.
	ConflictKeys []string
	// Rows is the data to upsert. All rows must have the same keys. Values are
	// formatted as SQL literals via FormatLiteral, supporting nil, string, bool,
	// integer, float, and time.Time types.
	Rows []map[string]any
	// RowsFile is the relative path to a JSONL file containing row data.
	// When set and Rows is empty, rows are loaded lazily at runtime from
	// filepath.Join(migrationsDir, RowsFile).
	RowsFile string
}

// TypeName returns the operation type identifier.
func (op *UpsertData) TypeName() string { return "upsert_data" }

// TableName returns the target table name.
func (op *UpsertData) TableName() string { return op.Table }

// IsDestructive returns false — upserting seed data is not considered destructive.
func (op *UpsertData) IsDestructive() bool { return false }

// Describe returns a human-readable description of this operation.
func (op *UpsertData) Describe() string {
	return fmt.Sprintf("Upsert %d row(s) into %s", len(op.Rows), op.Table)
}

// Up generates the upsert SQL by delegating to the provider's GenerateUpsert.
// Returns empty string when Rows is empty.
//
// DefaultRef values in rows are resolved through the defaults map: if the key
// is present, the resolved SQL expression is emitted verbatim (not quoted); if
// not, the DefaultRef string itself is used as a raw SQL expression.
func (op *UpsertData) Up(p providers.Provider, _ *SchemaState, defaults map[string]string, migrationsDir string) (string, error) {
	rows := op.Rows
	if len(rows) == 0 && op.RowsFile != "" {
		loaded, err := loadJSONLRows(filepath.Join(migrationsDir, op.RowsFile))
		if err != nil {
			return "", fmt.Errorf("loading rows from %s: %w", op.RowsFile, err)
		}
		rows = loaded
	}
	if len(rows) == 0 {
		return "", nil
	}

	// Determine a stable column order by sorting the keys of the first row.
	columns := SortedKeys(rows[0])

	// Pre-format every value as a SQL literal string, resolving any DefaultRef
	// values through the active defaults map.
	valueLiterals := make([][]string, len(rows))
	for i, row := range rows {
		rowLits := make([]string, len(columns))
		for j, col := range columns {
			rowLits[j] = formatUpsertValue(row[col], defaults)
		}
		valueLiterals[i] = rowLits
	}

	return p.GenerateUpsert(op.Table, op.ConflictKeys, columns, valueLiterals), nil
}

// formatUpsertValue formats a single row value for use in a GenerateUpsert call.
// DefaultRef values are resolved through the defaults map and emitted as raw SQL
// expressions; all other values are formatted as SQL literals via FormatLiteral.
func formatUpsertValue(v any, defaults map[string]string) string {
	if ref, ok := v.(DefaultRef); ok {
		key := string(ref)
		if resolved, found := defaults[key]; found {
			return resolved
		}
		// Not in defaults map — treat the ref string itself as a raw SQL expression.
		return key
	}
	return FormatLiteral(v)
}

// Down generates DELETE statements that remove each upserted row by matching
// on the ConflictKeys. Returns empty string when Rows or ConflictKeys is empty.
func (op *UpsertData) Down(p providers.Provider, _ *SchemaState, _ map[string]string, migrationsDir string) (string, error) {
	rows := op.Rows
	if len(rows) == 0 && op.RowsFile != "" {
		loaded, err := loadJSONLRows(filepath.Join(migrationsDir, op.RowsFile))
		if err != nil {
			return "", fmt.Errorf("loading rows from %s: %w", op.RowsFile, err)
		}
		rows = loaded
	}
	if len(rows) == 0 || len(op.ConflictKeys) == 0 {
		return "", nil
	}

	tbl := p.QuoteName(op.Table)
	stmts := make([]string, 0, len(rows))
	for _, row := range rows {
		conditions := make([]string, 0, len(op.ConflictKeys))
		for _, key := range op.ConflictKeys {
			conditions = append(conditions,
				fmt.Sprintf("%s = %s", p.QuoteName(key), FormatLiteral(row[key])))
		}
		stmts = append(stmts,
			fmt.Sprintf("DELETE FROM %s WHERE %s;", tbl, strings.Join(conditions, " AND ")))
	}
	return strings.Join(stmts, "\n"), nil
}

// Mutate is a no-op — UpsertData does not alter the schema state.
func (op *UpsertData) Mutate(_ *SchemaState) error { return nil }
