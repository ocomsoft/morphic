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

// Package codegen generates Go source files for the migration framework.
package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/ocomsoft/morphic/internal/yaml"
	"github.com/ocomsoft/morphic/migrate"
)

// SquashGenerator generates squashed migration .go files.
// A squashed migration combines multiple migrations into one, listing the originals
// in its Replaces field so the runner can skip them if already applied.
type SquashGenerator struct{}

// NewSquashGenerator creates a new SquashGenerator.
func NewSquashGenerator() *SquashGenerator {
	return &SquashGenerator{}
}

// GenerateSquash generates the source code for a squashed migration .go file.
// name is the new squashed migration name.
// replaces is the ordered list of migration names being replaced.
// migrations is the ordered list of Migration objects to squash.
func (g *SquashGenerator) GenerateSquash(
	name string,
	replaces []string,
	migrations []*migrate.Migration,
) (string, error) {
	var buf bytes.Buffer

	buf.WriteString("package main\n\n")
	buf.WriteString("import m \"github.com/ocomsoft/morphic/migrate\"\n\n")
	buf.WriteString("func init() {\n")
	fmt.Fprintf(&buf, "\tm.Register(&m.Migration{\n")
	fmt.Fprintf(&buf, "\t\tName:         %q,\n", name)
	buf.WriteString("\t\tDependencies: []string{},\n")

	// Replaces field — lists the migration names this squash replaces
	replaceStrs := make([]string, len(replaces))
	for i, r := range replaces {
		replaceStrs[i] = fmt.Sprintf("%q", r)
	}
	fmt.Fprintf(&buf, "\t\tReplaces:     []string{%s},\n", strings.Join(replaceStrs, ", "))

	// Combine all operations from all migrations
	buf.WriteString("\t\tOperations: []m.Operation{\n")
	for _, mig := range migrations {
		for _, op := range mig.Operations {
			opStr, err := renderOperation(op)
			if err != nil {
				return "", fmt.Errorf("rendering operation from %q: %w", mig.Name, err)
			}
			buf.WriteString(opStr)
		}
	}
	buf.WriteString("\t\t},\n")
	buf.WriteString("\t})\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("formatting squash migration: %w\nRaw:\n%s", err, buf.String())
	}
	return string(formatted), nil
}

// GenerateStarlarkSquash generates a .star squashed migration file.
func (g *SquashGenerator) GenerateStarlarkSquash(
	name string,
	replaces []string,
	migrations []*migrate.Migration,
) (string, error) {
	var b strings.Builder

	b.WriteString("migration(\n")
	fmt.Fprintf(&b, "    name = %q,\n", name)
	b.WriteString("    dependencies = [],\n")

	replaceStrs := make([]string, len(replaces))
	for i, r := range replaces {
		replaceStrs[i] = fmt.Sprintf("%q", r)
	}
	fmt.Fprintf(&b, "    replaces = [%s],\n", strings.Join(replaceStrs, ", "))

	b.WriteString("    operations = [\n")
	for _, mig := range migrations {
		for _, op := range mig.Operations {
			opStr, err := convertOperation(op)
			if err != nil {
				return "", fmt.Errorf("converting operation from %q to Starlark: %w", mig.Name, err)
			}
			b.WriteString(opStr)
		}
	}
	b.WriteString("    ],\n")
	b.WriteString(")\n")
	return b.String(), nil
}

// renderOperation converts a migrate.Operation back to Go source literal.
// It reuses the package-level generateFieldLiteral and generateIndexLiteral
// functions from go_generator.go since they work with yaml.Field/yaml.Index types.
func renderOperation(op migrate.Operation) (string, error) {
	switch o := op.(type) {
	case *migrate.CreateTable:
		return renderCreateTable(o), nil
	case *migrate.DropTable:
		return fmt.Sprintf("\t\t\t&m.DropTable{Name: %q},\n", o.Name), nil
	case *migrate.RenameTable:
		return fmt.Sprintf("\t\t\t&m.RenameTable{OldName: %q, NewName: %q},\n",
			o.OldName, o.NewName), nil
	case *migrate.AddField:
		return renderAddField(o), nil
	case *migrate.DropField:
		return fmt.Sprintf("\t\t\t&m.DropField{Table: %q, Field: %q},\n",
			o.Table, o.Field), nil
	case *migrate.RenameField:
		return fmt.Sprintf("\t\t\t&m.RenameField{Table: %q, OldName: %q, NewName: %q},\n",
			o.Table, o.OldName, o.NewName), nil
	case *migrate.AlterField:
		return renderAlterField(o), nil
	case *migrate.AddIndex:
		return renderAddIndex(o), nil
	case *migrate.DropIndex:
		return fmt.Sprintf("\t\t\t&m.DropIndex{Table: %q, Index: %q},\n",
			o.Table, o.Index), nil
	case *migrate.RunSQL:
		return fmt.Sprintf("\t\t\t&m.RunSQL{ForwardSQL: %q, BackwardSQL: %q},\n",
			o.ForwardSQL, o.BackwardSQL), nil
	default:
		return "", fmt.Errorf("unknown operation type %T", op)
	}
}

// renderCreateTable emits a &m.CreateTable{...} literal from a migrate.CreateTable.
func renderCreateTable(op *migrate.CreateTable) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t\t\t&m.CreateTable{\n\t\t\t\tName: %q,\n", op.Name)

	if len(op.Fields) > 0 {
		b.WriteString("\t\t\t\tFields: []m.Field{\n")
		for _, f := range op.Fields {
			b.WriteString("\t\t\t\t\t")
			b.WriteString(generateFieldLiteral(migrateFieldToYAML(f)))
			b.WriteString(",\n")
		}
		b.WriteString("\t\t\t\t},\n")
	}

	if len(op.Indexes) > 0 {
		b.WriteString("\t\t\t\tIndexes: []m.Index{\n")
		for _, idx := range op.Indexes {
			b.WriteString("\t\t\t\t\t")
			b.WriteString(generateIndexLiteral(migrateIndexToYAML(idx)))
			b.WriteString(",\n")
		}
		b.WriteString("\t\t\t\t},\n")
	}

	b.WriteString("\t\t\t},\n")
	return b.String()
}

// renderAddField emits a &m.AddField{...} literal from a migrate.AddField.
func renderAddField(op *migrate.AddField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t\t\t&m.AddField{\n\t\t\t\tTable: %q,\n", op.Table)
	b.WriteString("\t\t\t\tField: ")
	b.WriteString(generateFieldLiteral(migrateFieldToYAML(op.Field)))
	b.WriteString(",\n\t\t\t},\n")
	return b.String()
}

// renderAlterField emits a &m.AlterField{...} literal from a migrate.AlterField.
func renderAlterField(op *migrate.AlterField) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t\t\t&m.AlterField{\n\t\t\t\tTable: %q,\n", op.Table)
	b.WriteString("\t\t\t\tOldField: ")
	b.WriteString(generateFieldLiteral(migrateFieldToYAML(op.OldField)))
	b.WriteString(",\n\t\t\t\tNewField: ")
	b.WriteString(generateFieldLiteral(migrateFieldToYAML(op.NewField)))
	b.WriteString(",\n\t\t\t},\n")
	return b.String()
}

// renderAddIndex emits a &m.AddIndex{...} literal from a migrate.AddIndex.
func renderAddIndex(op *migrate.AddIndex) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\t\t\t&m.AddIndex{\n\t\t\t\tTable: %q,\n", op.Table)
	b.WriteString("\t\t\t\tIndex: ")
	b.WriteString(generateIndexLiteral(migrateIndexToYAML(op.Index)))
	b.WriteString(",\n\t\t\t},\n")
	return b.String()
}

// migrateFieldToYAML converts a migrate.Field (bool Nullable) to a yaml.Field
// (*bool Nullable) for reuse with the generateFieldLiteral function.
func migrateFieldToYAML(f migrate.Field) yaml.Field {
	nullable := f.Nullable
	yf := yaml.Field{
		Name:       f.Name,
		Type:       f.Type,
		PrimaryKey: f.PrimaryKey,
		Nullable:   &nullable,
		Default:    f.Default,
		Length:     f.Length,
		Precision:  f.Precision,
		Scale:      f.Scale,
		AutoCreate: f.AutoCreate,
		AutoUpdate: f.AutoUpdate,
	}
	if f.ForeignKey != nil {
		yf.ForeignKey = &yaml.ForeignKey{
			Table:    f.ForeignKey.Table,
			OnDelete: f.ForeignKey.OnDelete,
		}
	}
	if f.ManyToMany != nil {
		yf.ManyToMany = &yaml.ManyToMany{Table: f.ManyToMany.Table}
	}
	return yf
}

// migrateIndexToYAML converts a migrate.Index to a yaml.Index for reuse with
// the generateIndexLiteral function.
func migrateIndexToYAML(idx migrate.Index) yaml.Index {
	return yaml.Index{
		Name:   idx.Name,
		Fields: idx.Fields,
		Unique: idx.Unique,
	}
}
