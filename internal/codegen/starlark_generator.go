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

// starlark_generator.go produces .star migration scripts from yaml.SchemaDiff,
// mirroring GoGenerator but emitting Starlark syntax with named arguments.
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ocomsoft/morphic/internal/utils"
	"github.com/ocomsoft/morphic/internal/yaml"
)

// StarlarkGenerator produces .star source code for migration files.
type StarlarkGenerator struct{}

// NewStarlarkGenerator creates a new StarlarkGenerator instance.
func NewStarlarkGenerator() *StarlarkGenerator {
	return &StarlarkGenerator{}
}

// FileExtension returns ".star".
func (g *StarlarkGenerator) FileExtension() string {
	return ".star"
}

// GenerateBlank generates a blank Starlark migration with a TODO comment.
func (g *StarlarkGenerator) GenerateBlank(name string, deps []string) (string, error) {
	var b strings.Builder
	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [\n")
	b.WriteString("        # TODO: Add migration operations here.\n")
	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// GenerateMigration generates a .star file from a schema diff.
func (g *StarlarkGenerator) GenerateMigration(
	name string,
	deps []string,
	diff *yaml.SchemaDiff,
	currentSchema, previousSchema *yaml.Schema,
	decisions map[int]yaml.PromptResponse,
) (string, error) {
	if diff == nil || !diff.HasChanges {
		return "", fmt.Errorf("no changes to generate migration for")
	}

	var b strings.Builder

	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(deps))
	b.WriteString("    operations = [\n")

	for i, change := range diff.Changes {
		decision := decisions[i]
		schemaOnly := decision == yaml.PromptOmit
		review := decision == yaml.PromptReview
		ignoreErrors := decision == yaml.PromptIgnoreErrors

		op, err := g.generateOperation(change, currentSchema, previousSchema, schemaOnly, ignoreErrors)
		if err != nil {
			return "", fmt.Errorf("generating operation for change %s on %s: %w",
				change.Type, change.TableName, err)
		}
		if review {
			b.WriteString("        # REVIEW: destructive operation — verify before running\n")
		}
		b.WriteString(op)
	}

	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// generateOperation converts a single yaml.Change into Starlark source.
func (g *StarlarkGenerator) generateOperation(
	change yaml.Change,
	currentSchema, previousSchema *yaml.Schema,
	schemaOnly, ignoreErrors bool,
) (string, error) {
	switch change.Type {
	case yaml.ChangeTypeTableAdded:
		return g.generateCreateTable(change, schemaOnly)
	case yaml.ChangeTypeTableRemoved:
		return g.generateDropTable(change, schemaOnly, ignoreErrors)
	case yaml.ChangeTypeTableRenamed:
		return g.generateRenameTable(change)
	case yaml.ChangeTypeFieldAdded:
		return g.generateAddField(change, schemaOnly)
	case yaml.ChangeTypeFieldRemoved:
		return g.generateDropField(change, schemaOnly, ignoreErrors)
	case yaml.ChangeTypeFieldModified:
		return g.generateAlterField(change, currentSchema, previousSchema)
	case yaml.ChangeTypeFieldRenamed:
		return g.generateRenameField(change)
	case yaml.ChangeTypeIndexAdded:
		return g.generateAddIndex(change)
	case yaml.ChangeTypeIndexRemoved:
		return g.generateDropIndex(change, ignoreErrors)
	case yaml.ChangeTypeForeignKeyAdded:
		return g.generateAddForeignKey(change)
	case yaml.ChangeTypeForeignKeyRemoved:
		return g.generateDropForeignKey(change, ignoreErrors)
	case yaml.ChangeTypeDefaultsModified:
		return g.generateSetDefaults(change)
	case yaml.ChangeTypeTypeMappingsModified:
		return g.generateSetTypeMappings(change)
	default:
		return "", fmt.Errorf("unsupported change type: %s", change.Type)
	}
}

func (g *StarlarkGenerator) generateCreateTable(change yaml.Change, schemaOnly bool) (string, error) {
	table, ok := change.NewValue.(yaml.Table)
	if !ok {
		return "", fmt.Errorf("expected yaml.Table for NewValue, got %T", change.NewValue)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "        create_table(%q,\n", table.Name)

	if len(table.Fields) > 0 {
		b.WriteString("            fields = [\n")
		for _, f := range table.Fields {
			fmt.Fprintf(&b, "                %s,\n", generateStarlarkField(f))
		}
		b.WriteString("            ],\n")
	}

	// Indexes (exclude FromFK)
	var userIndexes []yaml.Index
	for _, idx := range table.Indexes {
		if !idx.FromFK {
			userIndexes = append(userIndexes, idx)
		}
	}
	if len(userIndexes) > 0 {
		b.WriteString("            indexes = [\n")
		for _, idx := range userIndexes {
			fmt.Fprintf(&b, "                %s,\n", generateStarlarkIndex(idx))
		}
		b.WriteString("            ],\n")
	}

	if schemaOnly {
		b.WriteString("            schema_only = True,\n")
	}

	b.WriteString("        ),\n")
	return b.String(), nil
}

func (g *StarlarkGenerator) generateDropTable(change yaml.Change, schemaOnly, ignoreErrors bool) (string, error) {
	var extras []string
	if schemaOnly {
		extras = append(extras, "schema_only=True")
	}
	if ignoreErrors {
		extras = append(extras, "ignore_errors=True")
	}
	extra := ""
	if len(extras) > 0 {
		extra = ", " + strings.Join(extras, ", ")
	}
	return fmt.Sprintf("        drop_table(%q%s),\n", change.TableName, extra), nil
}

func (g *StarlarkGenerator) generateRenameTable(change yaml.Change) (string, error) {
	newName, ok := change.NewValue.(string)
	if !ok {
		return "", fmt.Errorf("expected string for NewValue in table rename, got %T", change.NewValue)
	}
	return fmt.Sprintf("        rename_table(%q, %q),\n",
		change.TableName, newName), nil
}

func (g *StarlarkGenerator) generateAddField(change yaml.Change, schemaOnly bool) (string, error) {
	field, ok := change.NewValue.(yaml.Field)
	if !ok {
		return "", fmt.Errorf("expected yaml.Field for NewValue, got %T", change.NewValue)
	}
	extra := ""
	if schemaOnly {
		extra = ", schema_only=True"
	}
	return fmt.Sprintf("        add_field(%q, %s%s),\n",
		change.TableName, generateStarlarkField(field), extra), nil
}

func (g *StarlarkGenerator) generateDropField(change yaml.Change, schemaOnly, ignoreErrors bool) (string, error) {
	var extras []string
	if schemaOnly {
		extras = append(extras, "schema_only=True")
	}
	if ignoreErrors {
		extras = append(extras, "ignore_errors=True")
	}
	extra := ""
	if len(extras) > 0 {
		extra = ", " + strings.Join(extras, ", ")
	}
	return fmt.Sprintf("        drop_field(%q, %q%s),\n",
		change.TableName, change.FieldName, extra), nil
}

func (g *StarlarkGenerator) generateAlterField(
	change yaml.Change,
	currentSchema, previousSchema *yaml.Schema,
) (string, error) {
	var oldField, newField *yaml.Field

	if currentSchema != nil && previousSchema != nil {
		nf := lookupFieldInSchema(currentSchema, change.TableName, change.FieldName)
		newField = &nf
		of := lookupFieldInSchema(previousSchema, change.TableName, change.FieldName)
		oldField = &of
	}

	if oldField == nil {
		oldType := ""
		if s, ok := change.OldValue.(string); ok {
			oldType = s
		}
		f := yaml.Field{Name: change.FieldName, Type: oldType}
		oldField = &f
	}
	if newField == nil {
		newType := ""
		if s, ok := change.NewValue.(string); ok {
			newType = s
		}
		f := yaml.Field{Name: change.FieldName, Type: newType}
		newField = &f
	}

	return fmt.Sprintf("        alter_field(%q,\n            old_field = %s,\n            new_field = %s,\n        ),\n",
		change.TableName, generateStarlarkField(*oldField), generateStarlarkField(*newField)), nil
}

// lookupFieldInSchema finds a field by table and field name in a schema.
func lookupFieldInSchema(schema *yaml.Schema, tableName, fieldName string) yaml.Field {
	if schema == nil {
		return yaml.Field{Name: fieldName}
	}
	table := schema.GetTableByName(tableName)
	if table == nil {
		return yaml.Field{Name: fieldName}
	}
	f := table.GetFieldByName(fieldName)
	if f == nil {
		return yaml.Field{Name: fieldName}
	}
	return *f
}

func (g *StarlarkGenerator) generateRenameField(change yaml.Change) (string, error) {
	newName, ok := change.NewValue.(string)
	if !ok {
		return "", fmt.Errorf("expected string for NewValue in field rename, got %T", change.NewValue)
	}
	return fmt.Sprintf("        rename_field(%q, %q, %q),\n",
		change.TableName, change.FieldName, newName), nil
}

func (g *StarlarkGenerator) generateAddIndex(change yaml.Change) (string, error) {
	idx, ok := change.NewValue.(yaml.Index)
	if !ok {
		return "", fmt.Errorf("expected yaml.Index for NewValue, got %T", change.NewValue)
	}
	return fmt.Sprintf("        add_index(%q, %s),\n",
		change.TableName, generateStarlarkIndex(idx)), nil
}

func (g *StarlarkGenerator) generateDropIndex(change yaml.Change, ignoreErrors bool) (string, error) {
	if change.FieldName == "" {
		return "", fmt.Errorf("drop_index change for table %q has empty index name", change.TableName)
	}
	extra := ""
	if ignoreErrors {
		extra = ", ignore_errors=True"
	}
	return fmt.Sprintf("        drop_index(%q, %q%s),\n",
		change.TableName, change.FieldName, extra), nil
}

func (g *StarlarkGenerator) generateAddForeignKey(change yaml.Change) (string, error) {
	field, ok := change.NewValue.(yaml.Field)
	if !ok {
		return "", fmt.Errorf("expected yaml.Field for NewValue in FK added, got %T", change.NewValue)
	}
	if field.ForeignKey == nil {
		return "", fmt.Errorf("FK added change for %s.%s has nil ForeignKey", change.TableName, change.FieldName)
	}
	constraintName := utils.SafeConstraintName(fmt.Sprintf("fk_%s_%s", change.TableName, change.FieldName))
	onDelete := field.ForeignKey.OnDelete
	if onDelete == "" {
		onDelete = "PROTECT"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "        add_foreign_key(%q, %q, %q, %q,\n", change.TableName, change.FieldName, constraintName, field.ForeignKey.Table)
	fmt.Fprintf(&b, "            on_delete = %q,\n", onDelete)
	if field.ForeignKey.OnUpdate != "" {
		fmt.Fprintf(&b, "            on_update = %q,\n", field.ForeignKey.OnUpdate)
	}
	b.WriteString("            ignore_errors = True,\n")
	b.WriteString("        ),\n")
	return b.String(), nil
}

func (g *StarlarkGenerator) generateDropForeignKey(change yaml.Change, ignoreErrors bool) (string, error) {
	constraintName := utils.SafeConstraintName(fmt.Sprintf("fk_%s_%s", change.TableName, change.FieldName))
	extra := ""
	if ignoreErrors {
		extra = ", ignore_errors=True"
	}
	return fmt.Sprintf("        drop_foreign_key(%q, %q%s),\n",
		change.TableName, constraintName, extra), nil
}

func (g *StarlarkGenerator) generateSetDefaults(change yaml.Change) (string, error) {
	defaults, ok := change.NewValue.(map[string]string)
	if !ok {
		return "", fmt.Errorf("expected map[string]string for defaults NewValue, got %T", change.NewValue)
	}
	return fmt.Sprintf("        set_defaults(%s),\n", formatStarlarkStringDict(defaults)), nil
}

func (g *StarlarkGenerator) generateSetTypeMappings(change yaml.Change) (string, error) {
	mappings, ok := change.NewValue.(map[string]string)
	if !ok {
		return "", fmt.Errorf("expected map[string]string for type mappings NewValue, got %T", change.NewValue)
	}
	return fmt.Sprintf("        set_type_mappings(%s),\n", formatStarlarkStringDict(mappings)), nil
}

// ---------------------------------------------------------------------------
// Starlark literal helpers
// ---------------------------------------------------------------------------

// knownTypedFields lists field types that have dedicated typed builtins.
var knownTypedFields = map[string]bool{
	"uuid": true, "varchar": true, "text": true, "integer": true,
	"bigint": true, "boolean": true, "timestamp": true, "date": true,
	"float": true, "jsonb": true, "bytes": true, "decimal": true,
	"serial": true, "time": true, "foreign_key": true,
}

// generateStarlarkField outputs a typed field helper (e.g. uuid("id", ...))
// when possible, falling back to field("name", "type", ...) for unknown types.
func generateStarlarkField(f yaml.Field) string {
	if knownTypedFields[f.Type] {
		return generateTypedStarlarkField(f)
	}
	return generateGenericStarlarkField(f)
}

// generateTypedStarlarkField emits typed builtins like varchar("name", 255, ...).
func generateTypedStarlarkField(f yaml.Field) string {
	var positionalArgs string
	var kwargs []string

	switch f.Type {
	case "varchar":
		positionalArgs = fmt.Sprintf("%q, %d", f.Name, f.Length)
	case "decimal":
		positionalArgs = fmt.Sprintf("%q, %d, %d", f.Name, f.Precision, f.Scale)
	case "foreign_key":
		fkArg := "fk(\"\")"
		if f.ForeignKey != nil {
			fkArg = generateStarlarkFK(f.ForeignKey)
		}
		positionalArgs = fmt.Sprintf("%q, %s", f.Name, fkArg)
	default:
		positionalArgs = fmt.Sprintf("%q", f.Name)
	}

	nullable := f.Nullable == nil || *f.Nullable
	if nullable {
		kwargs = append(kwargs, "nullable=True")
	}
	if f.PrimaryKey {
		kwargs = append(kwargs, "primary_key=True")
	}
	if f.Default != "" {
		kwargs = append(kwargs, fmt.Sprintf("default=%q", f.Default))
	}
	// Length/precision/scale only as kwargs for types that don't take them positionally
	if f.Type != "varchar" && f.Length > 0 {
		kwargs = append(kwargs, fmt.Sprintf("length=%d", f.Length))
	}
	if f.Type != "decimal" && f.Precision > 0 {
		kwargs = append(kwargs, fmt.Sprintf("precision=%d", f.Precision))
	}
	if f.Type != "decimal" && f.Scale > 0 {
		kwargs = append(kwargs, fmt.Sprintf("scale=%d", f.Scale))
	}
	if f.AutoCreate {
		kwargs = append(kwargs, "auto_create=True")
	}
	if f.AutoUpdate {
		kwargs = append(kwargs, "auto_update=True")
	}
	if f.Type != "foreign_key" && f.ManyToMany != nil {
		kwargs = append(kwargs, fmt.Sprintf("many_to_many=%q", f.ManyToMany.Table))
	}

	args := positionalArgs
	if len(kwargs) > 0 {
		args += ", " + strings.Join(kwargs, ", ")
	}
	return fmt.Sprintf("%s(%s)", f.Type, args)
}

// generateGenericStarlarkField emits the generic field("name", "type", ...) form
// for types without a dedicated builtin.
func generateGenericStarlarkField(f yaml.Field) string {
	var kwargs []string

	if f.Nullable == nil || *f.Nullable {
		kwargs = append(kwargs, "nullable=True")
	}
	if f.PrimaryKey {
		kwargs = append(kwargs, "primary_key=True")
	}
	if f.Default != "" {
		kwargs = append(kwargs, fmt.Sprintf("default=%q", f.Default))
	}
	if f.Length > 0 {
		kwargs = append(kwargs, fmt.Sprintf("length=%d", f.Length))
	}
	if f.Precision > 0 {
		kwargs = append(kwargs, fmt.Sprintf("precision=%d", f.Precision))
	}
	if f.Scale > 0 {
		kwargs = append(kwargs, fmt.Sprintf("scale=%d", f.Scale))
	}
	if f.AutoCreate {
		kwargs = append(kwargs, "auto_create=True")
	}
	if f.AutoUpdate {
		kwargs = append(kwargs, "auto_update=True")
	}
	if f.ForeignKey != nil {
		kwargs = append(kwargs, fmt.Sprintf("foreign_key=%s", generateStarlarkFK(f.ForeignKey)))
	}
	if f.ManyToMany != nil {
		kwargs = append(kwargs, fmt.Sprintf("many_to_many=%q", f.ManyToMany.Table))
	}

	args := fmt.Sprintf("%q, %q", f.Name, f.Type)
	if len(kwargs) > 0 {
		args += ", " + strings.Join(kwargs, ", ")
	}
	return fmt.Sprintf("field(%s)", args)
}

// generateStarlarkFK outputs fk("table", on_delete="...", on_update="...").
func generateStarlarkFK(fk *yaml.ForeignKey) string {
	var kwargs []string
	if fk.OnDelete != "" {
		kwargs = append(kwargs, fmt.Sprintf("on_delete=%q", fk.OnDelete))
	}
	if fk.OnUpdate != "" {
		kwargs = append(kwargs, fmt.Sprintf("on_update=%q", fk.OnUpdate))
	}
	args := fmt.Sprintf("%q", fk.Table)
	if len(kwargs) > 0 {
		args += ", " + strings.Join(kwargs, ", ")
	}
	return fmt.Sprintf("fk(%s)", args)
}

// generateStarlarkIndex outputs index("name", ["f1", "f2"], unique=True, ...).
func generateStarlarkIndex(idx yaml.Index) string {
	fieldStrs := make([]string, len(idx.Fields))
	for i, f := range idx.Fields {
		fieldStrs[i] = fmt.Sprintf("%q", f)
	}
	fields := "[" + strings.Join(fieldStrs, ", ") + "]"

	var kwargs []string
	if idx.Unique {
		kwargs = append(kwargs, "unique=True")
	}
	if idx.Method != "" {
		kwargs = append(kwargs, fmt.Sprintf("method=%q", idx.Method))
	}
	if idx.Where != "" {
		kwargs = append(kwargs, fmt.Sprintf("where=%q", idx.Where))
	}

	args := fmt.Sprintf("%q, %s", idx.Name, fields)
	if len(kwargs) > 0 {
		args += ", " + strings.Join(kwargs, ", ")
	}
	return fmt.Sprintf("index(%s)", args)
}

// formatStarlarkDepsList formats a deps list like: "0001_initial", "0002_add_users"
func formatStarlarkDepsList(deps []string) string {
	if len(deps) == 0 {
		return ""
	}
	parts := make([]string, len(deps))
	for i, d := range deps {
		parts[i] = fmt.Sprintf("%q", d)
	}
	return strings.Join(parts, ", ")
}

// formatStarlarkStringDict formats a Go map[string]string as a Starlark dict literal.
func formatStarlarkStringDict(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%q: %q", k, m[k])
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
