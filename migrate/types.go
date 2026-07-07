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

// Package migrate provides the runtime library for the morphic Go migration framework.
// Generated migration files import this package and call Register() in their init() functions.
package migrate

// Migration represents a single database migration with its name, dependencies, and operations.
type Migration struct {
	Name         string      `json:"name"`               // Unique identifier e.g. "0001_initial"
	Dependencies []string    `json:"dependencies"`       // Names of migrations this depends on
	Operations   []Operation `json:"-"`                  // Ordered list of schema operations to apply
	Replaces     []string    `json:"replaces,omitempty"` // For squashed migrations: names of migrations this replaces
	Initial      bool        `json:"initial,omitempty"`  // Marks this as an initial migration for --fake-initial support
}

// Field represents a database column definition used in migration operations.
type Field struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"` // varchar, text, integer, uuid, boolean, timestamp, foreign_key, etc.
	PrimaryKey bool        `json:"primary_key,omitempty"`
	Nullable   bool        `json:"nullable,omitempty"`
	Default    string      `json:"default,omitempty"`     // default reference name e.g. "new_uuid", "now", "true"
	Length     int         `json:"length,omitempty"`      // for varchar
	Precision  int         `json:"precision,omitempty"`   // for decimal/numeric
	Scale      int         `json:"scale,omitempty"`       // for decimal/numeric
	AutoCreate bool        `json:"auto_create,omitempty"` // auto-set on row creation (created_at)
	AutoUpdate bool        `json:"auto_update,omitempty"` // auto-set on row update (updated_at)
	ForeignKey *ForeignKey `json:"foreign_key,omitempty"`
	ManyToMany *ManyToMany `json:"many_to_many,omitempty"`
}

// ForeignKey represents a foreign key constraint.
type ForeignKey struct {
	Table    string `json:"table"`
	OnDelete string `json:"on_delete,omitempty"`
	OnUpdate string `json:"on_update,omitempty"`
}

// ManyToMany represents a many-to-many relationship via junction table.
type ManyToMany struct {
	Table string `json:"table"`
}

// Index represents a database index definition.
type Index struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
	Unique bool     `json:"unique,omitempty"`
	// Method specifies the index access method (e.g. BTREE, HASH, GIN, GIST, BRIN).
	// Leave empty to use the database default. Not supported by SQLite or SQL Server.
	Method string `json:"method,omitempty"`
	// Where is a partial index predicate. Leave empty for a full index. Not supported by MySQL.
	Where string `json:"where,omitempty"`
	// FromFK marks indexes auto-generated for foreign key columns. These are
	// managed automatically by AddForeignKey/DropForeignKey and excluded from
	// schema diffs and generated migration code.
	FromFK bool `json:"from_fk,omitempty"`
}

// ForeignKeyConstraint represents a FK constraint tracked in SchemaState.
// Note: this is distinct from migrate.ForeignKey (the field-level FK metadata).
// ForeignKeyConstraint tracks what constraints exist in the database at runtime.
type ForeignKeyConstraint struct {
	Name            string `json:"name"`       // constraint name, e.g. fk_orders_user_id
	FieldName       string `json:"field_name"` // the column carrying the FK
	ReferencedTable string `json:"referenced_table"`
	OnDelete        string `json:"on_delete,omitempty"`
	OnUpdate        string `json:"on_update,omitempty"`
}
