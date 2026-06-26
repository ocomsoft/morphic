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

// starlark_builtins.go defines the Starlark builtin functions that migration
// .star scripts call. These functions convert Starlark values into migrate.*
// operation structs.
package interp

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/ocomsoft/morphic/migrate"
)

// ---------------------------------------------------------------------------
// opValue wraps a migrate.Operation as a starlark.Value so that operation
// builtins can return values that the migration() builtin later extracts.
// ---------------------------------------------------------------------------

// opValue wraps a migrate.Operation so it can be passed through Starlark
// as an opaque value. The migration() builtin type-asserts list elements
// to *opValue to collect the final Operation slice.
type opValue struct {
	op migrate.Operation
}

// Ensure *opValue satisfies starlark.Value at compile time.
var _ starlark.Value = (*opValue)(nil)

// String returns a human-readable representation of the wrapped operation.
func (o *opValue) String() string { return fmt.Sprintf("<op %s>", o.op.Describe()) }

// Type returns the Starlark type name.
func (o *opValue) Type() string { return "operation" }

// Freeze is a no-op — operation values are immutable once created.
func (o *opValue) Freeze() {}

// Truth always returns true — an operation value is always truthy.
func (o *opValue) Truth() starlark.Bool { return starlark.True }

// Hash returns an error — operation values are not hashable.
func (o *opValue) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: operation")
}

// ---------------------------------------------------------------------------
// StarlarkBuiltins holds all builtin functions injected into .star scripts.
// ---------------------------------------------------------------------------

// StarlarkBuiltins holds all builtin functions injected into .star scripts
// and collects the resulting Migration after script execution.
type StarlarkBuiltins struct {
	collected *migrate.Migration
}

// NewStarlarkBuiltins creates a new StarlarkBuiltins instance.
func NewStarlarkBuiltins() *StarlarkBuiltins {
	return &StarlarkBuiltins{}
}

// Collected returns the migration that was collected by the migration() builtin,
// or nil if migration() was never called.
func (b *StarlarkBuiltins) Collected() *migrate.Migration {
	return b.collected
}

// Env returns a starlark.StringDict containing all builtin functions,
// suitable for injection as the predeclared environment of a Starlark thread.
func (b *StarlarkBuiltins) Env() starlark.StringDict {
	env := starlark.StringDict{
		// Generic leaf builtins — return dicts.
		"field": starlark.NewBuiltin("field", b.builtinField),
		"fk":    starlark.NewBuiltin("fk", b.builtinFK),
		"index": starlark.NewBuiltin("index", b.builtinIndex),
		"row":   starlark.NewBuiltin("row", b.builtinRow),

		// Typed field builtins — return dicts with type pre-set.
		"uuid":        starlark.NewBuiltin("uuid", b.typedSimpleBuiltin("uuid")),
		"varchar":     starlark.NewBuiltin("varchar", b.typedVarcharBuiltin()),
		"text":        starlark.NewBuiltin("text", b.typedSimpleBuiltin("text")),
		"integer":     starlark.NewBuiltin("integer", b.typedSimpleBuiltin("integer")),
		"bigint":      starlark.NewBuiltin("bigint", b.typedSimpleBuiltin("bigint")),
		"boolean":     starlark.NewBuiltin("boolean", b.typedSimpleBuiltin("boolean")),
		"timestamp":   starlark.NewBuiltin("timestamp", b.typedDatetimeBuiltin("timestamp")),
		"date":        starlark.NewBuiltin("date", b.typedDatetimeBuiltin("date")),
		"float":       starlark.NewBuiltin("float", b.typedSimpleBuiltin("float")),
		"jsonb":       starlark.NewBuiltin("jsonb", b.typedSimpleBuiltin("jsonb")),
		"bytes":       starlark.NewBuiltin("bytes", b.typedSimpleBuiltin("bytes")),
		"decimal":     starlark.NewBuiltin("decimal", b.typedDecimalBuiltin()),
		"serial":      starlark.NewBuiltin("serial", b.typedSimpleBuiltin("serial")),
		"time":        starlark.NewBuiltin("time", b.typedDatetimeBuiltin("time")),
		"foreign_key": starlark.NewBuiltin("foreign_key", b.typedForeignKeyBuiltin()),

		// Operation builtins — return opValue.
		"migration":         starlark.NewBuiltin("migration", b.builtinMigration),
		"set_defaults":      starlark.NewBuiltin("set_defaults", b.builtinSetDefaults),
		"set_type_mappings": starlark.NewBuiltin("set_type_mappings", b.builtinSetTypeMappings),
		"create_table":      starlark.NewBuiltin("create_table", b.builtinCreateTable),
		"drop_table":        starlark.NewBuiltin("drop_table", b.builtinDropTable),
		"add_field":         starlark.NewBuiltin("add_field", b.builtinAddField),
		"drop_field":        starlark.NewBuiltin("drop_field", b.builtinDropField),
		"alter_field":       starlark.NewBuiltin("alter_field", b.builtinAlterField),
		"rename_field":      starlark.NewBuiltin("rename_field", b.builtinRenameField),
		"rename_table":      starlark.NewBuiltin("rename_table", b.builtinRenameTable),
		"add_index":         starlark.NewBuiltin("add_index", b.builtinAddIndex),
		"drop_index":        starlark.NewBuiltin("drop_index", b.builtinDropIndex),
		"add_foreign_key":   starlark.NewBuiltin("add_foreign_key", b.builtinAddForeignKey),
		"drop_foreign_key":  starlark.NewBuiltin("drop_foreign_key", b.builtinDropForeignKey),
		"upsert_data":       starlark.NewBuiltin("upsert_data", b.builtinUpsertData),
		"run_sql":           starlark.NewBuiltin("run_sql", b.builtinRunSQL),
	}
	return env
}

// ---------------------------------------------------------------------------
// Leaf builtins
// ---------------------------------------------------------------------------

// builtinField implements field(name, type, **kwargs) → *starlark.Dict.
// Positional: name (string), type (string).
// Keyword: nullable, primary_key, unique, default, length, precision, scale,
// auto_create, auto_update, foreign_key (dict from fk()), many_to_many (string).
func (b *StarlarkBuiltins) builtinField(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name       string
		typ        string
		nullable   bool
		primaryKey bool
		dflt       string
		length     int
		precision  int
		scale      int
		autoCreate bool
		autoUpdate bool
		foreignKey starlark.Value = starlark.None
		manyToMany string
	)

	if err := starlark.UnpackArgs("field", args, kwargs,
		"name", &name,
		"type", &typ,
		"nullable?", &nullable,
		"primary_key?", &primaryKey,
		"default?", &dflt,
		"length?", &length,
		"precision?", &precision,
		"scale?", &scale,
		"auto_create?", &autoCreate,
		"auto_update?", &autoUpdate,
		"foreign_key?", &foreignKey,
		"many_to_many?", &manyToMany,
	); err != nil {
		return nil, err
	}

	return buildFieldDict(name, typ, nullable, primaryKey, dflt, length, precision, scale, autoCreate, autoUpdate, foreignKey, manyToMany), nil
}

// builtinFK implements fk(table, **kwargs) → *starlark.Dict.
// Positional: table (string).
// Keyword: on_delete, on_update.
func (b *StarlarkBuiltins) builtinFK(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table    string
		onDelete string
		onUpdate string
	)
	if err := starlark.UnpackArgs("fk", args, kwargs,
		"table", &table,
		"on_delete?", &onDelete,
		"on_update?", &onUpdate,
	); err != nil {
		return nil, err
	}

	d := starlark.NewDict(3)
	mustSet(d, "table", starlark.String(table))
	mustSet(d, "on_delete", starlark.String(onDelete))
	mustSet(d, "on_update", starlark.String(onUpdate))
	return d, nil
}

// builtinIndex implements index(name, fields, **kwargs) → *starlark.Dict.
// Positional: name (string), fields (list of strings).
// Keyword: unique, method, where.
func (b *StarlarkBuiltins) builtinIndex(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name   string
		fields *starlark.List
		unique bool
		method string
		where  string
	)
	if err := starlark.UnpackArgs("index", args, kwargs,
		"name", &name,
		"fields", &fields,
		"unique?", &unique,
		"method?", &method,
		"where?", &where,
	); err != nil {
		return nil, err
	}

	d := starlark.NewDict(5)
	mustSet(d, "name", starlark.String(name))
	mustSet(d, "fields", fields)
	mustSet(d, "unique", starlark.Bool(unique))
	mustSet(d, "method", starlark.String(method))
	mustSet(d, "where", starlark.String(where))
	return d, nil
}

// builtinRow implements row(**kwargs) → *starlark.Dict.
// All arguments are keyword-only; each key-value pair becomes a dict entry.
func (b *StarlarkBuiltins) builtinRow(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("row: unexpected positional arguments")
	}
	d := starlark.NewDict(len(kwargs))
	for _, kv := range kwargs {
		if err := d.SetKey(starlark.String(string(kv[0].(starlark.String))), kv[1]); err != nil {
			return nil, fmt.Errorf("row: %w", err)
		}
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Typed field builtins — shorthand for field() with the type pre-set.
// ---------------------------------------------------------------------------

// typedSimpleBuiltin returns a builtin for non-datetime types: type_name(name, **kwargs).
// Works for uuid, text, integer, bigint, boolean, float, jsonb, bytes, serial.
// Does not accept auto_create/auto_update — those are only meaningful on datetime fields.
func (b *StarlarkBuiltins) typedSimpleBuiltin(typeName string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name       string
			nullable   bool
			primaryKey bool
			dflt       string
			length     int
		)
		if err := starlark.UnpackArgs(typeName, args, kwargs,
			"name", &name,
			"nullable?", &nullable,
			"primary_key?", &primaryKey,
			"default?", &dflt,
			"length?", &length,
		); err != nil {
			return nil, err
		}
		return buildFieldDict(name, typeName, nullable, primaryKey, dflt, length, 0, 0, false, false, starlark.None, ""), nil
	}
}

// typedDatetimeBuiltin returns a builtin for datetime types: type_name(name, **kwargs).
// Works for timestamp, date, time. Accepts auto_create/auto_update kwargs.
func (b *StarlarkBuiltins) typedDatetimeBuiltin(typeName string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name       string
			nullable   bool
			primaryKey bool
			dflt       string
			autoCreate bool
			autoUpdate bool
		)
		if err := starlark.UnpackArgs(typeName, args, kwargs,
			"name", &name,
			"nullable?", &nullable,
			"primary_key?", &primaryKey,
			"default?", &dflt,
			"auto_create?", &autoCreate,
			"auto_update?", &autoUpdate,
		); err != nil {
			return nil, err
		}
		return buildFieldDict(name, typeName, nullable, primaryKey, dflt, 0, 0, 0, autoCreate, autoUpdate, starlark.None, ""), nil
	}
}

// typedVarcharBuiltin returns a builtin for varchar(name, length, **kwargs).
func (b *StarlarkBuiltins) typedVarcharBuiltin() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name       string
			length     int
			nullable   bool
			primaryKey bool
			dflt       string
		)
		if err := starlark.UnpackArgs("varchar", args, kwargs,
			"name", &name,
			"length", &length,
			"nullable?", &nullable,
			"primary_key?", &primaryKey,
			"default?", &dflt,
		); err != nil {
			return nil, err
		}
		return buildFieldDict(name, "varchar", nullable, primaryKey, dflt, length, 0, 0, false, false, starlark.None, ""), nil
	}
}

// typedDecimalBuiltin returns a builtin for decimal(name, precision, scale, **kwargs).
func (b *StarlarkBuiltins) typedDecimalBuiltin() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name       string
			precision  int
			scale      int
			nullable   bool
			primaryKey bool
			dflt       string
		)
		if err := starlark.UnpackArgs("decimal", args, kwargs,
			"name", &name,
			"precision", &precision,
			"scale", &scale,
			"nullable?", &nullable,
			"primary_key?", &primaryKey,
			"default?", &dflt,
		); err != nil {
			return nil, err
		}
		return buildFieldDict(name, "decimal", nullable, primaryKey, dflt, 0, precision, scale, false, false, starlark.None, ""), nil
	}
}

// typedForeignKeyBuiltin returns a builtin for foreign_key(name, fk_dict, **kwargs).
func (b *StarlarkBuiltins) typedForeignKeyBuiltin() func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var (
			name       string
			fkDict     starlark.Value = starlark.None
			nullable   bool
			primaryKey bool
			dflt       string
		)
		if err := starlark.UnpackArgs("foreign_key", args, kwargs,
			"name", &name,
			"fk", &fkDict,
			"nullable?", &nullable,
			"primary_key?", &primaryKey,
			"default?", &dflt,
		); err != nil {
			return nil, err
		}
		return buildFieldDict(name, "foreign_key", nullable, primaryKey, dflt, 0, 0, 0, false, false, fkDict, ""), nil
	}
}

// buildFieldDict constructs a field dict from typed parameters.
func buildFieldDict(name, typ string, nullable, primaryKey bool, dflt string, length, precision, scale int, autoCreate, autoUpdate bool, foreignKey starlark.Value, manyToMany string) *starlark.Dict {
	d := starlark.NewDict(11)
	mustSet(d, "name", starlark.String(name))
	mustSet(d, "type", starlark.String(typ))
	mustSet(d, "nullable", starlark.Bool(nullable))
	mustSet(d, "primary_key", starlark.Bool(primaryKey))
	mustSet(d, "default", starlark.String(dflt))
	mustSet(d, "length", starlark.MakeInt(length))
	mustSet(d, "precision", starlark.MakeInt(precision))
	mustSet(d, "scale", starlark.MakeInt(scale))
	mustSet(d, "auto_create", starlark.Bool(autoCreate))
	mustSet(d, "auto_update", starlark.Bool(autoUpdate))
	if foreignKey != starlark.None {
		mustSet(d, "foreign_key", foreignKey)
	}
	if manyToMany != "" {
		mustSet(d, "many_to_many", starlark.String(manyToMany))
	}
	return d
}

// ---------------------------------------------------------------------------
// Operation builtins
// ---------------------------------------------------------------------------

// builtinMigration implements migration(name, dependencies=[], operations=[]).
// It extracts opValue entries from the operations list and stores the result
// in b.collected.
func (b *StarlarkBuiltins) builtinMigration(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name         string
		dependencies *starlark.List
		operations   *starlark.List
	)
	if err := starlark.UnpackArgs("migration", args, kwargs,
		"name", &name,
		"dependencies?", &dependencies,
		"operations?", &operations,
	); err != nil {
		return nil, err
	}

	m := &migrate.Migration{Name: name}

	// Extract dependencies.
	if dependencies != nil {
		for i := 0; i < dependencies.Len(); i++ {
			s, ok := dependencies.Index(i).(starlark.String)
			if !ok {
				return nil, fmt.Errorf("migration: dependencies[%d] must be a string, got %s", i, dependencies.Index(i).Type())
			}
			m.Dependencies = append(m.Dependencies, string(s))
		}
	}

	// Extract operations.
	if operations != nil {
		for i := 0; i < operations.Len(); i++ {
			ov, ok := operations.Index(i).(*opValue)
			if !ok {
				return nil, fmt.Errorf("migration: operations[%d] must be an operation, got %s", i, operations.Index(i).Type())
			}
			m.Operations = append(m.Operations, ov.op)
		}
	}

	b.collected = m
	return starlark.None, nil
}

// builtinSetDefaults implements set_defaults(mapping) → opValue.
func (b *StarlarkBuiltins) builtinSetDefaults(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var mapping *starlark.Dict
	if err := starlark.UnpackPositionalArgs("set_defaults", args, kwargs, 1, &mapping); err != nil {
		return nil, err
	}
	m, err := stringDictToMap(mapping)
	if err != nil {
		return nil, fmt.Errorf("set_defaults: %w", err)
	}
	return &opValue{op: &migrate.SetDefaults{Defaults: m}}, nil
}

// builtinSetTypeMappings implements set_type_mappings(mapping) → opValue.
func (b *StarlarkBuiltins) builtinSetTypeMappings(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var mapping *starlark.Dict
	if err := starlark.UnpackPositionalArgs("set_type_mappings", args, kwargs, 1, &mapping); err != nil {
		return nil, err
	}
	m, err := stringDictToMap(mapping)
	if err != nil {
		return nil, fmt.Errorf("set_type_mappings: %w", err)
	}
	return &opValue{op: &migrate.SetTypeMappings{TypeMappings: m}}, nil
}

// builtinCreateTable implements create_table(name, fields=[], indexes=[]) → opValue.
func (b *StarlarkBuiltins) builtinCreateTable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name    string
		fields  *starlark.List
		indexes *starlark.List
	)
	if err := starlark.UnpackArgs("create_table", args, kwargs,
		"name", &name,
		"fields?", &fields,
		"indexes?", &indexes,
	); err != nil {
		return nil, err
	}

	op := &migrate.CreateTable{Name: name}

	if fields != nil {
		for i := 0; i < fields.Len(); i++ {
			d, ok := fields.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("create_table: fields[%d] must be a dict (from field()), got %s", i, fields.Index(i).Type())
			}
			f, err := dictToField(d)
			if err != nil {
				return nil, fmt.Errorf("create_table: fields[%d]: %w", i, err)
			}
			op.Fields = append(op.Fields, f)
		}
	}

	if indexes != nil {
		for i := 0; i < indexes.Len(); i++ {
			d, ok := indexes.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("create_table: indexes[%d] must be a dict (from index()), got %s", i, indexes.Index(i).Type())
			}
			idx, err := dictToIndex(d)
			if err != nil {
				return nil, fmt.Errorf("create_table: indexes[%d]: %w", i, err)
			}
			op.Indexes = append(op.Indexes, idx)
		}
	}

	return &opValue{op: op}, nil
}

// builtinDropTable implements drop_table(name, schema_only=False, ignore_errors=False) → opValue.
func (b *StarlarkBuiltins) builtinDropTable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name         string
		schemaOnly   bool
		ignoreErrors bool
	)
	if err := starlark.UnpackArgs("drop_table", args, kwargs,
		"name", &name,
		"schema_only?", &schemaOnly,
		"ignore_errors?", &ignoreErrors,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.DropTable{Name: name, SchemaOnly: schemaOnly, IgnoreErrors: ignoreErrors}}, nil
}

// builtinAddField implements add_field(table, field_dict) → opValue.
func (b *StarlarkBuiltins) builtinAddField(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table     string
		fieldDict *starlark.Dict
	)
	if err := starlark.UnpackArgs("add_field", args, kwargs,
		"table", &table,
		"field", &fieldDict,
	); err != nil {
		return nil, err
	}
	f, err := dictToField(fieldDict)
	if err != nil {
		return nil, fmt.Errorf("add_field: %w", err)
	}
	return &opValue{op: &migrate.AddField{Table: table, Field: f}}, nil
}

// builtinDropField implements drop_field(table, field_name, schema_only=False, ignore_errors=False) → opValue.
func (b *StarlarkBuiltins) builtinDropField(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table        string
		fieldName    string
		schemaOnly   bool
		ignoreErrors bool
	)
	if err := starlark.UnpackArgs("drop_field", args, kwargs,
		"table", &table,
		"field_name", &fieldName,
		"schema_only?", &schemaOnly,
		"ignore_errors?", &ignoreErrors,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.DropField{Table: table, Field: fieldName, SchemaOnly: schemaOnly, IgnoreErrors: ignoreErrors}}, nil
}

// builtinAlterField implements alter_field(table, old_field, new_field) → opValue.
// Both old_field and new_field are dicts produced by the field() builtin.
func (b *StarlarkBuiltins) builtinAlterField(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table    string
		oldField *starlark.Dict
		newField *starlark.Dict
	)
	if err := starlark.UnpackArgs("alter_field", args, kwargs,
		"table", &table,
		"old_field", &oldField,
		"new_field", &newField,
	); err != nil {
		return nil, err
	}
	of, err := dictToField(oldField)
	if err != nil {
		return nil, fmt.Errorf("alter_field: old_field: %w", err)
	}
	nf, err := dictToField(newField)
	if err != nil {
		return nil, fmt.Errorf("alter_field: new_field: %w", err)
	}
	return &opValue{op: &migrate.AlterField{Table: table, OldField: of, NewField: nf}}, nil
}

// builtinRenameField implements rename_field(table, old_name, new_name) → opValue.
func (b *StarlarkBuiltins) builtinRenameField(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table   string
		oldName string
		newName string
	)
	if err := starlark.UnpackArgs("rename_field", args, kwargs,
		"table", &table,
		"old_name", &oldName,
		"new_name", &newName,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.RenameField{Table: table, OldName: oldName, NewName: newName}}, nil
}

// builtinRenameTable implements rename_table(old_name, new_name) → opValue.
func (b *StarlarkBuiltins) builtinRenameTable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		oldName string
		newName string
	)
	if err := starlark.UnpackArgs("rename_table", args, kwargs,
		"old_name", &oldName,
		"new_name", &newName,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.RenameTable{OldName: oldName, NewName: newName}}, nil
}

// builtinAddIndex implements add_index(table, index_dict) → opValue.
func (b *StarlarkBuiltins) builtinAddIndex(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table   string
		idxDict *starlark.Dict
	)
	if err := starlark.UnpackArgs("add_index", args, kwargs,
		"table", &table,
		"index", &idxDict,
	); err != nil {
		return nil, err
	}
	idx, err := dictToIndex(idxDict)
	if err != nil {
		return nil, fmt.Errorf("add_index: %w", err)
	}
	return &opValue{op: &migrate.AddIndex{Table: table, Index: idx}}, nil
}

// builtinDropIndex implements drop_index(table, index_name, ignore_errors=False) → opValue.
func (b *StarlarkBuiltins) builtinDropIndex(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table        string
		indexName    string
		ignoreErrors bool
	)
	if err := starlark.UnpackArgs("drop_index", args, kwargs,
		"table", &table,
		"index_name", &indexName,
		"ignore_errors?", &ignoreErrors,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.DropIndex{Table: table, Index: indexName, IgnoreErrors: ignoreErrors}}, nil
}

// builtinAddForeignKey implements add_foreign_key(table, field_name, constraint_name,
// referenced_table, on_delete="CASCADE", on_update="") → opValue.
func (b *StarlarkBuiltins) builtinAddForeignKey(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table           string
		fieldName       string
		constraintName  string
		referencedTable string
		onDelete        = "CASCADE"
		onUpdate        string
		ignoreErrors    bool
	)
	if err := starlark.UnpackArgs("add_foreign_key", args, kwargs,
		"table", &table,
		"field_name", &fieldName,
		"constraint_name", &constraintName,
		"referenced_table", &referencedTable,
		"on_delete?", &onDelete,
		"on_update?", &onUpdate,
		"ignore_errors?", &ignoreErrors,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.AddForeignKey{
		Table:           table,
		FieldName:       fieldName,
		ConstraintName:  constraintName,
		ReferencedTable: referencedTable,
		OnDelete:        onDelete,
		OnUpdate:        onUpdate,
		IgnoreErrors:    ignoreErrors,
	}}, nil
}

// builtinDropForeignKey implements drop_foreign_key(table, constraint_name, ignore_errors=False) → opValue.
func (b *StarlarkBuiltins) builtinDropForeignKey(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table          string
		constraintName string
		ignoreErrors   bool
	)
	if err := starlark.UnpackArgs("drop_foreign_key", args, kwargs,
		"table", &table,
		"constraint_name", &constraintName,
		"ignore_errors?", &ignoreErrors,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.DropForeignKey{Table: table, ConstraintName: constraintName, IgnoreErrors: ignoreErrors}}, nil
}

// builtinUpsertData implements upsert_data(table, conflict_keys, rows?, rows_file?) → opValue.
// Either rows (a list of dicts) or rows_file (a string path) must be provided, not both.
func (b *StarlarkBuiltins) builtinUpsertData(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		table        string
		conflictKeys *starlark.List
		rows         *starlark.List
		rowsFile     string
	)
	if err := starlark.UnpackArgs("upsert_data", args, kwargs,
		"table", &table,
		"conflict_keys", &conflictKeys,
		"rows?", &rows,
		"rows_file?", &rowsFile,
	); err != nil {
		return nil, err
	}

	hasRows := rows != nil && rows.Len() > 0
	hasRowsFile := rowsFile != ""

	if hasRows && hasRowsFile {
		return nil, fmt.Errorf("upsert_data: cannot specify both rows and rows_file")
	}
	if !hasRows && !hasRowsFile {
		return nil, fmt.Errorf("upsert_data: must specify either rows or rows_file")
	}

	ck, err := stringListToSlice(conflictKeys)
	if err != nil {
		return nil, fmt.Errorf("upsert_data: conflict_keys: %w", err)
	}

	var goRows []map[string]any
	if hasRows {
		for i := 0; i < rows.Len(); i++ {
			d, ok := rows.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("upsert_data: rows[%d] must be a dict, got %s", i, rows.Index(i).Type())
			}
			goRows = append(goRows, dictToGoMap(d))
		}
	}

	return &opValue{op: &migrate.UpsertData{
		Table:        table,
		ConflictKeys: ck,
		Rows:         goRows,
		RowsFile:     rowsFile,
	}}, nil
}

// builtinRunSQL implements run_sql(forward, backward="") → opValue.
func (b *StarlarkBuiltins) builtinRunSQL(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		forward  string
		backward string
	)
	if err := starlark.UnpackArgs("run_sql", args, kwargs,
		"forward", &forward,
		"backward?", &backward,
	); err != nil {
		return nil, err
	}
	return &opValue{op: &migrate.RunSQL{ForwardSQL: forward, BackwardSQL: backward}}, nil
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

// dictToField converts a *starlark.Dict (produced by the field() builtin)
// into a migrate.Field.
func dictToField(d *starlark.Dict) (migrate.Field, error) {
	f := migrate.Field{}

	if v, ok := dictGetString(d, "name"); ok {
		f.Name = v
	} else {
		return f, fmt.Errorf("field dict missing required key 'name'")
	}

	if v, ok := dictGetString(d, "type"); ok {
		f.Type = v
	} else {
		return f, fmt.Errorf("field dict missing required key 'type'")
	}

	f.Nullable = dictGetBool(d, "nullable")
	f.PrimaryKey = dictGetBool(d, "primary_key")

	if v, ok := dictGetString(d, "default"); ok {
		f.Default = v
	}
	f.Length = dictGetInt(d, "length")
	f.Precision = dictGetInt(d, "precision")
	f.Scale = dictGetInt(d, "scale")
	f.AutoCreate = dictGetBool(d, "auto_create")
	f.AutoUpdate = dictGetBool(d, "auto_update")

	// foreign_key is a nested dict from fk().
	if v, found, _ := d.Get(starlark.String("foreign_key")); found && v != nil && v != starlark.None {
		fkDict, ok := v.(*starlark.Dict)
		if !ok {
			return f, fmt.Errorf("field 'foreign_key' must be a dict (from fk()), got %s", v.Type())
		}
		fk, err := dictToForeignKey(fkDict)
		if err != nil {
			return f, fmt.Errorf("field foreign_key: %w", err)
		}
		f.ForeignKey = fk
	}

	// many_to_many is a simple string (table name).
	if v, ok := dictGetString(d, "many_to_many"); ok && v != "" {
		f.ManyToMany = &migrate.ManyToMany{Table: v}
	}

	return f, nil
}

// dictToIndex converts a *starlark.Dict (produced by the index() builtin)
// into a migrate.Index.
func dictToIndex(d *starlark.Dict) (migrate.Index, error) {
	idx := migrate.Index{}

	if v, ok := dictGetString(d, "name"); ok {
		idx.Name = v
	} else {
		return idx, fmt.Errorf("index dict missing required key 'name'")
	}

	// fields is a starlark.List of strings.
	v, found, _ := d.Get(starlark.String("fields"))
	if !found || v == nil || v == starlark.None {
		return idx, fmt.Errorf("index dict missing required key 'fields'")
	}
	fieldsList, ok := v.(*starlark.List)
	if !ok {
		return idx, fmt.Errorf("index 'fields' must be a list, got %s", v.Type())
	}
	fields, err := stringListToSlice(fieldsList)
	if err != nil {
		return idx, fmt.Errorf("index fields: %w", err)
	}
	idx.Fields = fields

	idx.Unique = dictGetBool(d, "unique")
	if v, ok := dictGetString(d, "method"); ok {
		idx.Method = v
	}
	if v, ok := dictGetString(d, "where"); ok {
		idx.Where = v
	}

	return idx, nil
}

// dictToForeignKey converts a *starlark.Dict (produced by the fk() builtin)
// into a *migrate.ForeignKey.
func dictToForeignKey(d *starlark.Dict) (*migrate.ForeignKey, error) {
	fk := &migrate.ForeignKey{}

	if v, ok := dictGetString(d, "table"); ok {
		fk.Table = v
	} else {
		return nil, fmt.Errorf("fk dict missing required key 'table'")
	}

	if v, ok := dictGetString(d, "on_delete"); ok {
		fk.OnDelete = v
	}
	if v, ok := dictGetString(d, "on_update"); ok {
		fk.OnUpdate = v
	}

	return fk, nil
}

// starlarkToGoValue converts a starlark.Value to a native Go value.
// Handles String, Int, Float, Bool, None, Dict, and List.
func starlarkToGoValue(v starlark.Value) any {
	switch x := v.(type) {
	case starlark.String:
		return string(x)
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			return i
		}
		// Fall back to string representation for very large integers.
		return x.String()
	case starlark.Float:
		return float64(x)
	case starlark.Bool:
		return bool(x)
	case *starlark.NoneType:
		return nil
	case *starlark.Dict:
		return dictToGoMap(x)
	case *starlark.List:
		return listToGoSlice(x)
	default:
		return v.String()
	}
}

// dictToGoMap converts a *starlark.Dict to a map[string]any.
func dictToGoMap(d *starlark.Dict) map[string]any {
	m := make(map[string]any, d.Len())
	for _, item := range d.Items() {
		key := string(item[0].(starlark.String))
		m[key] = starlarkToGoValue(item[1])
	}
	return m
}

// listToGoSlice converts a *starlark.List to a []any.
func listToGoSlice(l *starlark.List) []any {
	s := make([]any, l.Len())
	for i := 0; i < l.Len(); i++ {
		s[i] = starlarkToGoValue(l.Index(i))
	}
	return s
}

// stringDictToMap converts a *starlark.Dict with string keys and string values
// to a map[string]string. Returns an error if any key or value is not a string.
func stringDictToMap(d *starlark.Dict) (map[string]string, error) {
	m := make(map[string]string, d.Len())
	for _, item := range d.Items() {
		k, ok := item[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("dict key must be a string, got %s", item[0].Type())
		}
		v, ok := item[1].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("dict value for key %q must be a string, got %s", k, item[1].Type())
		}
		m[string(k)] = string(v)
	}

	// Return a stable nil for empty maps to match Go convention.
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

// stringListToSlice converts a *starlark.List of strings to a []string.
func stringListToSlice(l *starlark.List) ([]string, error) {
	result := make([]string, l.Len())
	for i := 0; i < l.Len(); i++ {
		s, ok := l.Index(i).(starlark.String)
		if !ok {
			return nil, fmt.Errorf("element [%d] must be a string, got %s", i, l.Index(i).Type())
		}
		result[i] = string(s)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Dict access helpers — safe getters that return zero values for missing keys.
// ---------------------------------------------------------------------------

// dictGetString returns the string value for a key in a Starlark dict, or ("", false).
func dictGetString(d *starlark.Dict, key string) (string, bool) {
	v, found, _ := d.Get(starlark.String(key))
	if !found || v == nil || v == starlark.None {
		return "", false
	}
	s, ok := v.(starlark.String)
	if !ok {
		return "", false
	}
	return string(s), true
}

// dictGetBool returns the bool value for a key in a Starlark dict, or false.
func dictGetBool(d *starlark.Dict, key string) bool {
	v, found, _ := d.Get(starlark.String(key))
	if !found || v == nil {
		return false
	}
	b, ok := v.(starlark.Bool)
	if !ok {
		return false
	}
	return bool(b)
}

// dictGetInt returns the int value for a key in a Starlark dict, or 0.
func dictGetInt(d *starlark.Dict, key string) int {
	v, found, _ := d.Get(starlark.String(key))
	if !found || v == nil {
		return 0
	}
	si, ok := v.(starlark.Int)
	if !ok {
		return 0
	}
	i, _ := si.Int64()
	return int(i)
}

// mustSet sets a key-value pair on a starlark.Dict and panics on error.
// This is safe to use only for dicts we just created (no frozen dicts).
func mustSet(d *starlark.Dict, key string, val starlark.Value) {
	if err := d.SetKey(starlark.String(key), val); err != nil {
		panic(fmt.Sprintf("mustSet(%q): %v", key, err))
	}
}
