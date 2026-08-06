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

// schema_builtins.go defines Starlark builtins for the schema DSL (.star
// schema files). These builtins let users declare databases, tables, defaults,
// type mappings, and includes — collecting everything into a *types.Schema.
package interp

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/ocomsoft/morphic/internal/types"
)

// ---------------------------------------------------------------------------
// SchemaDSLBuiltins collects schema declarations from a .star schema file.
// ---------------------------------------------------------------------------

// SchemaDSLBuiltins holds the Starlark builtins for schema .star files and
// accumulates the resulting types.Schema as each builtin is called.
type SchemaDSLBuiltins struct {
	schema *types.Schema
}

// NewSchemaDSLBuiltins creates a new SchemaDSLBuiltins with an empty schema.
func NewSchemaDSLBuiltins() *SchemaDSLBuiltins {
	return &SchemaDSLBuiltins{
		schema: &types.Schema{},
	}
}

// Collected returns the accumulated schema after script execution.
func (s *SchemaDSLBuiltins) Collected() *types.Schema {
	return s.schema
}

// Env returns a starlark.StringDict containing all schema DSL builtins plus
// the shared field/index/fk leaf builtins from StarlarkBuiltins.
func (s *SchemaDSLBuiltins) Env() starlark.StringDict {
	// Start with the shared field/index/fk builtins from migration builtins.
	migBuiltins := NewStarlarkBuiltins()
	migEnv := migBuiltins.Env()

	// Keys we want to keep from the migration builtins (leaf helpers only).
	leafKeys := []string{
		"field", "fk", "index", "unique_index",
		"uuid", "varchar", "text", "integer", "bigint", "boolean",
		"timestamp", "date", "float", "jsonb", "bytes", "decimal",
		"serial", "time", "foreign_key",
	}

	env := make(starlark.StringDict, len(leafKeys)+5)
	for _, k := range leafKeys {
		if v, ok := migEnv[k]; ok {
			env[k] = v
		}
	}

	// Schema-specific builtins.
	env["database"] = starlark.NewBuiltin("database", s.builtinDatabase)
	env["table"] = starlark.NewBuiltin("table", s.builtinTable)
	env["defaults"] = starlark.NewBuiltin("defaults", s.builtinDefaults)
	env["type_mappings"] = starlark.NewBuiltin("type_mappings", s.builtinTypeMappings)
	env["include"] = starlark.NewBuiltin("include", s.builtinInclude)

	return env
}

// ---------------------------------------------------------------------------
// Schema builtins
// ---------------------------------------------------------------------------

// builtinDatabase implements database(name, version) — sets schema.Database.
func (s *SchemaDSLBuiltins) builtinDatabase(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, version string
	if err := starlark.UnpackArgs("database", args, kwargs,
		"name", &name,
		"version?", &version,
	); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("database: name must not be empty")
	}
	s.schema.Database = types.Database{
		Name:    name,
		Version: version,
	}
	return starlark.None, nil
}

// builtinTable implements table(name, fields=[], indexes=[]) — appends a table
// to the schema. Fields are dicts from field builtins, indexes from index().
func (s *SchemaDSLBuiltins) builtinTable(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name    string
		fields  *starlark.List
		indexes *starlark.List
	)
	if err := starlark.UnpackArgs("table", args, kwargs,
		"name", &name,
		"fields?", &fields,
		"indexes?", &indexes,
	); err != nil {
		return nil, err
	}

	table := types.Table{Name: name}

	if fields != nil {
		for i := 0; i < fields.Len(); i++ {
			d, ok := fields.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("table: fields[%d] must be a dict (from field()), got %s", i, fields.Index(i).Type())
			}
			f, err := schemaDictToField(d)
			if err != nil {
				return nil, fmt.Errorf("table: fields[%d]: %w", i, err)
			}
			table.Fields = append(table.Fields, f)
		}
	}

	if indexes != nil {
		for i := 0; i < indexes.Len(); i++ {
			d, ok := indexes.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("table: indexes[%d] must be a dict (from index()), got %s", i, indexes.Index(i).Type())
			}
			idx, err := schemaDictToIndex(d)
			if err != nil {
				return nil, fmt.Errorf("table: indexes[%d]: %w", i, err)
			}
			table.Indexes = append(table.Indexes, idx)
		}
	}

	s.schema.Tables = append(s.schema.Tables, table)
	return starlark.None, nil
}

// builtinDefaults implements defaults(db_type, mapping) — sets defaults for
// a database type (e.g. defaults("postgresql", {"blank": "”"})).
func (s *SchemaDSLBuiltins) builtinDefaults(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		dbType  string
		mapping *starlark.Dict
	)
	if err := starlark.UnpackArgs("defaults", args, kwargs,
		"db_type", &dbType,
		"mapping", &mapping,
	); err != nil {
		return nil, err
	}

	m, err := stringDictToMap(mapping)
	if err != nil {
		return nil, fmt.Errorf("defaults: %w", err)
	}

	if s.schema.Defaults == nil {
		s.schema.Defaults = make(types.Defaults)
	}
	s.schema.Defaults.SetForProvider(types.DatabaseType(dbType), m)
	return starlark.None, nil
}

// builtinTypeMappings implements type_mappings(db_type, mapping) — sets type
// mapping overrides for a database type.
func (s *SchemaDSLBuiltins) builtinTypeMappings(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		dbType  string
		mapping *starlark.Dict
	)
	if err := starlark.UnpackArgs("type_mappings", args, kwargs,
		"db_type", &dbType,
		"mapping", &mapping,
	); err != nil {
		return nil, err
	}

	m, err := stringDictToMap(mapping)
	if err != nil {
		return nil, fmt.Errorf("type_mappings: %w", err)
	}

	if s.schema.TypeMappings == nil {
		s.schema.TypeMappings = make(types.TypeMappings)
	}
	s.schema.TypeMappings.SetForProvider(types.DatabaseType(dbType), m)
	return starlark.None, nil
}

// builtinInclude implements include(module, path) — adds an include entry.
func (s *SchemaDSLBuiltins) builtinInclude(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var module, path string
	if err := starlark.UnpackArgs("include", args, kwargs,
		"module", &module,
		"path", &path,
	); err != nil {
		return nil, err
	}
	s.schema.Include = append(s.schema.Include, types.Include{
		Module: module,
		Path:   path,
	})
	return starlark.None, nil
}

// ---------------------------------------------------------------------------
// Conversion helpers — Starlark dicts to types.* structs
// ---------------------------------------------------------------------------

// schemaDictToField converts a *starlark.Dict (from field builtins) into a
// types.Field. Similar to dictToField but targets types.Field not migrate.Field.
func schemaDictToField(d *starlark.Dict) (types.Field, error) {
	var f types.Field

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

	nullable := dictGetBool(d, "nullable")
	f.Nullable = &nullable
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
		fk, err := schemaDictToForeignKey(fkDict)
		if err != nil {
			return f, fmt.Errorf("field foreign_key: %w", err)
		}
		f.ForeignKey = fk
	}

	// many_to_many is a simple string (table name).
	if v, ok := dictGetString(d, "many_to_many"); ok && v != "" {
		f.ManyToMany = &types.ManyToMany{Table: v}
	}

	return f, nil
}

// schemaDictToIndex converts a *starlark.Dict (from the index() builtin) into
// a types.Index.
func schemaDictToIndex(d *starlark.Dict) (types.Index, error) {
	var idx types.Index

	if v, ok := dictGetString(d, "name"); ok {
		idx.Name = v
	} else {
		return idx, fmt.Errorf("index dict missing required key 'name'")
	}

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

// schemaDictToForeignKey converts a *starlark.Dict (from the fk() builtin)
// into a *types.ForeignKey.
func schemaDictToForeignKey(d *starlark.Dict) (*types.ForeignKey, error) {
	fk := &types.ForeignKey{}

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
