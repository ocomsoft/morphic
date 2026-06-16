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

// starlark_converter.go converts in-memory migrate.Migration objects (loaded
// via Yaegi) into Starlark .star source files. This enables format conversion
// from Go migrations to Starlark without re-parsing Go source text.
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ocomsoft/morphic/internal/yaml"
	"github.com/ocomsoft/morphic/migrate"
)

// ConvertMigrationToStarlark takes a migrate.Migration (typically loaded via
// Yaegi from a .go file) and returns the equivalent .star source code.
func ConvertMigrationToStarlark(m *migrate.Migration) (string, error) {
	var b strings.Builder

	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", m.Name)
	fmt.Fprintf(&b, "    dependencies = [%s],\n", formatStarlarkDepsList(m.Dependencies))
	b.WriteString("    operations = [\n")

	for _, op := range m.Operations {
		src, err := convertOperation(op)
		if err != nil {
			return "", fmt.Errorf("converting operation %T in migration %s: %w", op, m.Name, err)
		}
		b.WriteString(src)
	}

	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// convertOperation dispatches a single migrate.Operation to its Starlark emitter.
func convertOperation(op migrate.Operation) (string, error) {
	switch o := op.(type) {
	case *migrate.SetDefaults:
		return convertSetDefaults(o), nil
	case *migrate.SetTypeMappings:
		return convertSetTypeMappings(o), nil
	case *migrate.CreateTable:
		return convertCreateTable(o), nil
	case *migrate.DropTable:
		return convertDropTable(o), nil
	case *migrate.RenameTable:
		return convertRenameTable(o), nil
	case *migrate.AddField:
		return convertAddField(o), nil
	case *migrate.DropField:
		return convertDropField(o), nil
	case *migrate.AlterField:
		return convertAlterField(o), nil
	case *migrate.RenameField:
		return convertRenameField(o), nil
	case *migrate.AddIndex:
		return convertAddIndex(o), nil
	case *migrate.DropIndex:
		return convertDropIndex(o), nil
	case *migrate.AddForeignKey:
		return convertAddForeignKey(o), nil
	case *migrate.DropForeignKey:
		return convertDropForeignKey(o), nil
	case *migrate.UpsertData:
		return convertUpsertData(o), nil
	case *migrate.RunSQL:
		return convertRunSQL(o), nil
	default:
		return "", fmt.Errorf("unsupported operation type: %T", op)
	}
}

func convertSetDefaults(op *migrate.SetDefaults) string {
	return fmt.Sprintf("        set_defaults(%s),\n", formatStarlarkStringDict(op.Defaults))
}

func convertSetTypeMappings(op *migrate.SetTypeMappings) string {
	return fmt.Sprintf("        set_type_mappings(%s),\n", formatStarlarkStringDict(op.TypeMappings))
}

func convertCreateTable(op *migrate.CreateTable) string {
	var b strings.Builder
	fmt.Fprintf(&b, "        create_table(%q,\n", op.Name)

	if len(op.Fields) > 0 {
		b.WriteString("            fields = [\n")
		for _, f := range op.Fields {
			fmt.Fprintf(&b, "                %s,\n", migrateFieldToStarlark(f))
		}
		b.WriteString("            ],\n")
	}

	var userIndexes []migrate.Index
	for _, idx := range op.Indexes {
		if !idx.FromFK {
			userIndexes = append(userIndexes, idx)
		}
	}
	if len(userIndexes) > 0 {
		b.WriteString("            indexes = [\n")
		for _, idx := range userIndexes {
			fmt.Fprintf(&b, "                %s,\n", migrateIndexToStarlark(idx))
		}
		b.WriteString("            ],\n")
	}

	if op.SchemaOnly {
		b.WriteString("            schema_only = True,\n")
	}

	b.WriteString("        ),\n")
	return b.String()
}

func convertDropTable(op *migrate.DropTable) string {
	var extras []string
	if op.SchemaOnly {
		extras = append(extras, "schema_only=True")
	}
	if op.IgnoreErrors {
		extras = append(extras, "ignore_errors=True")
	}
	extra := ""
	if len(extras) > 0 {
		extra = ", " + strings.Join(extras, ", ")
	}
	return fmt.Sprintf("        drop_table(%q%s),\n", op.Name, extra)
}

func convertRenameTable(op *migrate.RenameTable) string {
	return fmt.Sprintf("        rename_table(%q, %q),\n", op.OldName, op.NewName)
}

func convertAddField(op *migrate.AddField) string {
	extra := ""
	if op.SchemaOnly {
		extra = ", schema_only=True"
	}
	return fmt.Sprintf("        add_field(%q, %s%s),\n",
		op.Table, migrateFieldToStarlark(op.Field), extra)
}

func convertDropField(op *migrate.DropField) string {
	var extras []string
	if op.SchemaOnly {
		extras = append(extras, "schema_only=True")
	}
	if op.IgnoreErrors {
		extras = append(extras, "ignore_errors=True")
	}
	extra := ""
	if len(extras) > 0 {
		extra = ", " + strings.Join(extras, ", ")
	}
	return fmt.Sprintf("        drop_field(%q, %q%s),\n",
		op.Table, op.Field, extra)
}

func convertAlterField(op *migrate.AlterField) string {
	return fmt.Sprintf("        alter_field(%q,\n            old_field = %s,\n            new_field = %s,\n        ),\n",
		op.Table, migrateFieldToStarlark(op.OldField), migrateFieldToStarlark(op.NewField))
}

func convertRenameField(op *migrate.RenameField) string {
	return fmt.Sprintf("        rename_field(%q, %q, %q),\n",
		op.Table, op.OldName, op.NewName)
}

func convertAddIndex(op *migrate.AddIndex) string {
	return fmt.Sprintf("        add_index(%q, %s),\n",
		op.Table, migrateIndexToStarlark(op.Index))
}

func convertDropIndex(op *migrate.DropIndex) string {
	extra := ""
	if op.IgnoreErrors {
		extra = ", ignore_errors=True"
	}
	return fmt.Sprintf("        drop_index(%q, %q%s),\n",
		op.Table, op.Index, extra)
}

func convertAddForeignKey(op *migrate.AddForeignKey) string {
	var b strings.Builder
	fmt.Fprintf(&b, "        add_foreign_key(%q, %q, %q, %q,\n", op.Table, op.FieldName, op.ConstraintName, op.ReferencedTable)
	if op.OnDelete != "" {
		fmt.Fprintf(&b, "            on_delete = %q,\n", op.OnDelete)
	}
	if op.OnUpdate != "" {
		fmt.Fprintf(&b, "            on_update = %q,\n", op.OnUpdate)
	}
	if op.IgnoreErrors {
		b.WriteString("            ignore_errors = True,\n")
	}
	b.WriteString("        ),\n")
	return b.String()
}

func convertDropForeignKey(op *migrate.DropForeignKey) string {
	extra := ""
	if op.IgnoreErrors {
		extra = ", ignore_errors=True"
	}
	return fmt.Sprintf("        drop_foreign_key(%q, %q%s),\n",
		op.Table, op.ConstraintName, extra)
}

func convertUpsertData(op *migrate.UpsertData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "        upsert_data(%q,\n", op.Table)

	conflictStrs := make([]string, len(op.ConflictKeys))
	for i, k := range op.ConflictKeys {
		conflictStrs[i] = fmt.Sprintf("%q", k)
	}
	fmt.Fprintf(&b, "            conflict_keys = [%s],\n", strings.Join(conflictStrs, ", "))

	if op.DataFile != "" {
		fmt.Fprintf(&b, "            file = %q,\n", op.DataFile)
	} else {
		b.WriteString("            rows = [\n")
		for _, row := range op.Rows {
			b.WriteString("                row(")
			keys := make([]string, 0, len(row))
			for k := range row {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				v := row[k]
				parts = append(parts, fmt.Sprintf("%s=%s", k, formatStarlarkLiteral(v)))
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("),\n")
		}
		b.WriteString("            ],\n")
	}
	b.WriteString("        ),\n")
	return b.String()
}

func convertRunSQL(op *migrate.RunSQL) string {
	var b strings.Builder
	b.WriteString("        run_sql(\n")
	fmt.Fprintf(&b, "            forward = %s,\n", formatStarlarkTripleQuote(op.ForwardSQL))
	if op.BackwardSQL != "" {
		fmt.Fprintf(&b, "            backward = %s,\n", formatStarlarkTripleQuote(op.BackwardSQL))
	}
	b.WriteString("        ),\n")
	return b.String()
}

// ---------------------------------------------------------------------------
// Helper functions for converting migrate types to Starlark literals
// ---------------------------------------------------------------------------

// migrateFieldToStarlark converts a migrate.Field to a Starlark field() call.
func migrateFieldToStarlark(f migrate.Field) string {
	yf := yaml.Field{
		Name:       f.Name,
		Type:       f.Type,
		PrimaryKey: f.PrimaryKey,
		Default:    f.Default,
		Length:     f.Length,
		Precision:  f.Precision,
		Scale:      f.Scale,
		AutoCreate: f.AutoCreate,
		AutoUpdate: f.AutoUpdate,
	}
	if f.Nullable {
		yf.Nullable = boolPtrCodegen(true)
	} else {
		yf.Nullable = boolPtrCodegen(false)
	}
	if f.ForeignKey != nil {
		yf.ForeignKey = &yaml.ForeignKey{
			Table:    f.ForeignKey.Table,
			OnDelete: f.ForeignKey.OnDelete,
			OnUpdate: f.ForeignKey.OnUpdate,
		}
	}
	if f.ManyToMany != nil {
		yf.ManyToMany = &yaml.ManyToMany{Table: f.ManyToMany.Table}
	}
	return generateStarlarkField(yf)
}

func boolPtrCodegen(b bool) *bool { return &b }

// migrateIndexToStarlark converts a migrate.Index to a Starlark index() call.
func migrateIndexToStarlark(idx migrate.Index) string {
	return generateStarlarkIndex(yaml.Index{
		Name:   idx.Name,
		Fields: idx.Fields,
		Unique: idx.Unique,
		Method: idx.Method,
		Where:  idx.Where,
	})
}

// formatStarlarkLiteral formats any value as a Starlark literal for row data.
func formatStarlarkLiteral(v any) string {
	if v == nil {
		return "None"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		if val {
			return "True"
		}
		return "False"
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case migrate.DefaultRef:
		return fmt.Sprintf("default_ref(%q)", string(val))
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", v))
	}
}

// formatStarlarkTripleQuote wraps SQL in a Starlark string literal.
// Multi-line SQL uses triple-quoted strings; single-line SQL with special
// characters uses Go-style %q escaping (Starlark accepts the same escapes).
func formatStarlarkTripleQuote(sql string) string {
	if sql == "" {
		return `""`
	}
	if strings.Contains(sql, "\n") {
		// Multi-line: use triple quotes, but escape trailing " to avoid """"
		s := sql
		if strings.HasSuffix(s, `"`) {
			s = s[:len(s)-1] + `\"`
		}
		return `"""` + s + `"""`
	}
	// Single-line: %q handles all escaping including embedded quotes
	return fmt.Sprintf("%q", sql)
}
