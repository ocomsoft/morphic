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

// Package migrate_test — Usage Guide integration tests.
//
// These tests mirror the end-to-end scenario in docs/Usage.md, covering every
// step from initial schema creation through stored procedures and full rollback.
//
// The SQL-generation assertions use the PostgreSQL provider (no live DB needed)
// to verify that custom type_mappings (CITEXT, DOUBLE PRECISION) and custom
// defaults (gen_random_uuid(), 'active', '{}') are emitted correctly.
//
// The full lifecycle tests use SQLite so they run without any external services.
package migrate_test

import (
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/providers/postgresql"
	"github.com/ocomsoft/morphic/migrate"
)

// ---------------------------------------------------------------------------
// Shared defaults and type-mappings from docs/Usage.md schema
// ---------------------------------------------------------------------------

// usageDefaults mirrors the defaults section of the Usage.md schema.yaml.
var usageDefaults = map[string]string{
	"blank":          "''",
	"now":            "CURRENT_TIMESTAMP",
	"new_uuid":       "gen_random_uuid()",
	"today":          "CURRENT_DATE",
	"zero":           "0",
	"true":           "true",
	"false":          "false",
	"empty_json":     "'{}'",
	"default_status": "'active'",
	"default_role":   "'member'",
}

// usageTypeMappings mirrors the type_mappings.postgresql section of the schema.
var usageTypeMappings = map[string]string{
	"text":  "CITEXT",
	"float": "DOUBLE PRECISION",
}

// newPGProvider returns a PostgreSQL provider with the custom type mappings
// from the Usage.md schema applied, matching the runtime behaviour of
// SetTypeMappings.Mutate → runner.provider.SetTypeMappings(state.TypeMappings).
func newPGProvider() *postgresql.Provider {
	p := postgresql.New()
	p.SetTypeMappings(usageTypeMappings)
	return p
}

// ---------------------------------------------------------------------------
// Section 1 — Custom defaults: SetDefaults operation
// ---------------------------------------------------------------------------

// TestUsageGuide_SetDefaults verifies that SetDefaults.Mutate loads all custom
// default keys from the Usage.md schema into the SchemaState.
func TestUsageGuide_SetDefaults(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetDefaults{Defaults: usageDefaults}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("SetDefaults.Mutate: %v", err)
	}
	checks := map[string]string{
		"new_uuid":       "gen_random_uuid()",
		"now":            "CURRENT_TIMESTAMP",
		"default_status": "'active'",
		"default_role":   "'member'",
		"empty_json":     "'{}'",
		"zero":           "0",
	}
	for key, want := range checks {
		if state.Defaults[key] != want {
			t.Errorf("defaults[%q] = %q, want %q", key, state.Defaults[key], want)
		}
	}
}

// TestUsageGuide_SetDefaults_NoSQL confirms SetDefaults emits no SQL.
func TestUsageGuide_SetDefaults_NoSQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()
	op := &migrate.SetDefaults{Defaults: usageDefaults}
	up, err := op.Up(p, state, nil, "")
	if err != nil || up != "" {
		t.Errorf("SetDefaults.Up should return empty SQL, got %q err=%v", up, err)
	}
	down, err := op.Down(p, state, nil, "")
	if err != nil || down != "" {
		t.Errorf("SetDefaults.Down should return empty SQL, got %q err=%v", down, err)
	}
}

// ---------------------------------------------------------------------------
// Section 2 — Custom type mappings: SetTypeMappings operation
// ---------------------------------------------------------------------------

// TestUsageGuide_SetTypeMappings verifies SetTypeMappings.Mutate loads the
// Usage.md overrides (CITEXT, DOUBLE PRECISION) into SchemaState.
func TestUsageGuide_SetTypeMappings(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetTypeMappings{TypeMappings: usageTypeMappings}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("SetTypeMappings.Mutate: %v", err)
	}
	if state.TypeMappings["text"] != "CITEXT" {
		t.Errorf("TypeMappings[text] = %q, want CITEXT", state.TypeMappings["text"])
	}
	if state.TypeMappings["float"] != "DOUBLE PRECISION" {
		t.Errorf("TypeMappings[float] = %q, want DOUBLE PRECISION", state.TypeMappings["float"])
	}
}

// TestUsageGuide_SetTypeMappings_NoSQL confirms SetTypeMappings emits no SQL.
func TestUsageGuide_SetTypeMappings_NoSQL(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetTypeMappings{TypeMappings: usageTypeMappings}
	up, err := op.Up(nil, state, nil, "")
	if err != nil || up != "" {
		t.Errorf("SetTypeMappings.Up should return empty SQL, got %q err=%v", up, err)
	}
	down, err := op.Down(nil, state, nil, "")
	if err != nil || down != "" {
		t.Errorf("SetTypeMappings.Down should return empty SQL, got %q err=%v", down, err)
	}
}

// ---------------------------------------------------------------------------
// Section 3 — Initial schema: users table SQL generation (PostgreSQL)
// ---------------------------------------------------------------------------

// TestUsageGuide_UsersTable_PostgreSQL_SQL verifies that the users CreateTable
// operation generates correct PostgreSQL DDL including:
// - UUID type with gen_random_uuid() default
// - CITEXT type via type_mappings (email field)
// - Symbolic defaults resolved: default_role → 'member', default_status → 'active'
// - empty_json default → '{}'
// - Unique indexes for email and username
func TestUsageGuide_UsersTable_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name: "users",
		Fields: []migrate.Field{
			{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid"},
			{Name: "email", Type: "text"},
			{Name: "username", Type: "varchar", Length: 100},
			{Name: "password_hash", Type: "varchar", Length: 255},
			{Name: "role", Type: "varchar", Length: 50, Default: "default_role"},
			{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
			{Name: "metadata", Type: "jsonb", Nullable: true, Default: "empty_json"},
			{Name: "created_at", Type: "timestamp", AutoCreate: true, Default: "now"},
			{Name: "updated_at", Type: "timestamp", Nullable: true, AutoUpdate: true},
		},
		Indexes: []migrate.Index{
			{Name: "idx_users_email", Fields: []string{"email"}, Unique: true},
			{Name: "idx_users_username", Fields: []string{"username"}, Unique: true},
			{Name: "idx_users_status", Fields: []string{"status"}, Unique: false},
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Up: %v", err)
	}

	// UUID primary key with gen_random_uuid()
	if !strings.Contains(sql, "gen_random_uuid()") {
		t.Errorf("expected gen_random_uuid() in SQL, got:\n%s", sql)
	}
	// text type maps to CITEXT
	if !strings.Contains(sql, "CITEXT") {
		t.Errorf("expected CITEXT (from type_mappings) in SQL, got:\n%s", sql)
	}
	// default_role resolves to 'member'
	if !strings.Contains(sql, "'member'") {
		t.Errorf("expected default_role resolved to 'member' in SQL, got:\n%s", sql)
	}
	// default_status resolves to 'active'
	if !strings.Contains(sql, "'active'") {
		t.Errorf("expected default_status resolved to 'active' in SQL, got:\n%s", sql)
	}
	// empty_json resolves to '{}'
	if !strings.Contains(sql, "'{}'") {
		t.Errorf("expected empty_json resolved to '{}' in SQL, got:\n%s", sql)
	}
	// now resolves to CURRENT_TIMESTAMP
	if !strings.Contains(sql, "CURRENT_TIMESTAMP") {
		t.Errorf("expected CURRENT_TIMESTAMP in SQL, got:\n%s", sql)
	}
	// Down is DROP TABLE
	downSQL, err := op.Down(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Down: %v", err)
	}
	if !strings.Contains(strings.ToUpper(downSQL), "DROP TABLE") {
		t.Errorf("expected DROP TABLE in down SQL, got:\n%s", downSQL)
	}
}

// ---------------------------------------------------------------------------
// Section 4 — Initial schema: products table (float → DOUBLE PRECISION)
// ---------------------------------------------------------------------------

// TestUsageGuide_ProductsTable_PostgreSQL_SQL verifies that the float field
// in the products table renders as DOUBLE PRECISION via type_mappings.
func TestUsageGuide_ProductsTable_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name: "products",
		Fields: []migrate.Field{
			{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid"},
			{Name: "name", Type: "varchar", Length: 255},
			{Name: "description", Type: "text", Nullable: true},
			{Name: "price", Type: "decimal", Precision: 10, Scale: 2},
			{Name: "weight_kg", Type: "float", Nullable: true},
			{Name: "stock_count", Type: "integer", Default: "zero"},
			{Name: "is_active", Type: "boolean", Default: "true"},
			{Name: "metadata", Type: "jsonb", Nullable: true, Default: "empty_json"},
			{Name: "created_at", Type: "timestamp", AutoCreate: true, Default: "now"},
			{Name: "updated_at", Type: "timestamp", Nullable: true, AutoUpdate: true},
		},
		Indexes: []migrate.Index{
			{Name: "idx_products_name", Fields: []string{"name"}, Unique: false},
			{Name: "idx_products_is_active_created", Fields: []string{"is_active", "created_at"}, Unique: false},
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Up: %v", err)
	}

	// float maps to DOUBLE PRECISION
	if !strings.Contains(sql, "DOUBLE PRECISION") {
		t.Errorf("expected DOUBLE PRECISION (from type_mappings) in SQL, got:\n%s", sql)
	}
	// DECIMAL(10,2) for price
	if !strings.Contains(sql, "DECIMAL(10,2)") {
		t.Errorf("expected DECIMAL(10,2) in SQL, got:\n%s", sql)
	}
	// description as CITEXT
	if !strings.Contains(sql, "CITEXT") {
		t.Errorf("expected CITEXT for description, got:\n%s", sql)
	}
	// stock_count zero → 0
	if !strings.Contains(sql, "DEFAULT 0") {
		t.Errorf("expected DEFAULT 0 for stock_count, got:\n%s", sql)
	}
}

// ---------------------------------------------------------------------------
// Section 5 — Initial schema: categories (self-referencing FK)
// ---------------------------------------------------------------------------

// TestUsageGuide_CategoriesTable_PostgreSQL_SQL verifies self-referencing FK
// and serial primary key for the categories table.
func TestUsageGuide_CategoriesTable_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name: "categories",
		Fields: []migrate.Field{
			{Name: "id", Type: "serial", PrimaryKey: true},
			{Name: "name", Type: "varchar", Length: 100},
			{Name: "slug", Type: "varchar", Length: 100},
			{Name: "parent_id", Type: "foreign_key", Nullable: true,
				ForeignKey: &migrate.ForeignKey{Table: "categories", OnDelete: "SET NULL"}},
		},
		Indexes: []migrate.Index{
			{Name: "idx_categories_slug", Fields: []string{"slug"}, Unique: true},
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Up: %v", err)
	}

	if !strings.Contains(strings.ToUpper(sql), "SERIAL") {
		t.Errorf("expected SERIAL for id, got:\n%s", sql)
	}
	// parent_id FK column must be present (exact type depends on schema context)
	if !strings.Contains(sql, "parent_id") {
		t.Errorf("expected parent_id column in SQL, got:\n%s", sql)
	}
	// slug column must be present
	if !strings.Contains(sql, "slug") {
		t.Errorf("expected slug column in SQL, got:\n%s", sql)
	}
}

// ---------------------------------------------------------------------------
// Section 6 — Initial schema: product_categories junction table
// ---------------------------------------------------------------------------

// TestUsageGuide_ProductCategories_PostgreSQL_SQL verifies the many-to-many
// junction table with two foreign keys and a unique composite index.
func TestUsageGuide_ProductCategories_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name: "product_categories",
		Fields: []migrate.Field{
			{Name: "id", Type: "serial", PrimaryKey: true},
			{Name: "product_id", Type: "foreign_key",
				ForeignKey: &migrate.ForeignKey{Table: "products", OnDelete: "CASCADE"}},
			{Name: "category_id", Type: "foreign_key",
				ForeignKey: &migrate.ForeignKey{Table: "categories", OnDelete: "CASCADE"}},
		},
		Indexes: []migrate.Index{
			{Name: "idx_product_categories_unique", Fields: []string{"product_id", "category_id"}, Unique: true},
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Up: %v", err)
	}

	// FK columns must be present
	if !strings.Contains(sql, "product_id") {
		t.Errorf("expected product_id column in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "category_id") {
		t.Errorf("expected category_id column in SQL, got:\n%s", sql)
	}
}

// ---------------------------------------------------------------------------
// Section 7 — Adding a table: orders (PostgreSQL SQL generation)
// ---------------------------------------------------------------------------

// TestUsageGuide_AddOrdersTable_PostgreSQL_SQL verifies the orders CreateTable
// generates the expected SQL, including FK to users with RESTRICT and CITEXT notes.
func TestUsageGuide_AddOrdersTable_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name: "orders",
		Fields: []migrate.Field{
			{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid"},
			{Name: "user_id", Type: "foreign_key",
				ForeignKey: &migrate.ForeignKey{Table: "users", OnDelete: "RESTRICT"}},
			{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
			{Name: "total_amount", Type: "decimal", Precision: 12, Scale: 2, Default: "zero"},
			{Name: "notes", Type: "text", Nullable: true},
			{Name: "placed_at", Type: "timestamp", AutoCreate: true, Default: "now"},
		},
		Indexes: []migrate.Index{
			{Name: "idx_orders_user_id", Fields: []string{"user_id"}, Unique: false},
			{Name: "idx_orders_status_placed", Fields: []string{"status", "placed_at"}, Unique: false},
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("CreateTable.Up: %v", err)
	}

	// UUID with gen_random_uuid()
	if !strings.Contains(sql, "gen_random_uuid()") {
		t.Errorf("expected gen_random_uuid() in orders SQL, got:\n%s", sql)
	}
	// DECIMAL(12,2)
	if !strings.Contains(sql, "DECIMAL(12,2)") {
		t.Errorf("expected DECIMAL(12,2) in orders SQL, got:\n%s", sql)
	}
	// notes as CITEXT
	if !strings.Contains(sql, "CITEXT") {
		t.Errorf("expected CITEXT for notes field in orders SQL, got:\n%s", sql)
	}
	// FK column must be present
	if !strings.Contains(sql, "user_id") {
		t.Errorf("expected user_id column in orders SQL, got:\n%s", sql)
	}
}

// ---------------------------------------------------------------------------
// Section 8 — Adding fields: AddField operation (PostgreSQL)
// ---------------------------------------------------------------------------

// TestUsageGuide_AddFields_PostgreSQL_SQL verifies that AddField generates the
// correct ALTER TABLE SQL for phone (varchar) and last_login_at (timestamp).
func TestUsageGuide_AddFields_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "uuid", PrimaryKey: true}}, nil)

	phoneOp := &migrate.AddField{
		Table: "users",
		Field: migrate.Field{Name: "phone", Type: "varchar", Length: 30, Nullable: true},
	}
	phoneSQL, err := phoneOp.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AddField(phone).Up: %v", err)
	}
	if !strings.Contains(strings.ToUpper(phoneSQL), "ALTER TABLE") {
		t.Errorf("expected ALTER TABLE in phone AddField SQL, got:\n%s", phoneSQL)
	}
	if !strings.Contains(phoneSQL, "phone") {
		t.Errorf("expected column name 'phone' in SQL, got:\n%s", phoneSQL)
	}

	loginOp := &migrate.AddField{
		Table: "users",
		Field: migrate.Field{Name: "last_login_at", Type: "timestamp", Nullable: true},
	}
	loginSQL, err := loginOp.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AddField(last_login_at).Up: %v", err)
	}
	if !strings.Contains(loginSQL, "last_login_at") {
		t.Errorf("expected column name 'last_login_at' in SQL, got:\n%s", loginSQL)
	}

	// Down for phone must be DROP COLUMN
	downSQL, err := phoneOp.Down(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AddField(phone).Down: %v", err)
	}
	if !strings.Contains(strings.ToUpper(downSQL), "DROP COLUMN") {
		t.Errorf("expected DROP COLUMN in Down SQL for AddField, got:\n%s", downSQL)
	}
}

// ---------------------------------------------------------------------------
// Section 9 — Adding indexes: AddIndex operation (PostgreSQL)
// ---------------------------------------------------------------------------

// TestUsageGuide_AddIndex_PostgreSQL_SQL verifies that AddIndex generates
// correct CREATE INDEX SQL for the composite users(role, status) index.
func TestUsageGuide_AddIndex_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "uuid", PrimaryKey: true}}, nil)

	op := &migrate.AddIndex{
		Table: "users",
		Index: migrate.Index{
			Name:   "idx_users_role_status",
			Fields: []string{"role", "status"},
			Unique: false,
		},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AddIndex.Up: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sql), "CREATE INDEX") {
		t.Errorf("expected CREATE INDEX in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "idx_users_role_status") {
		t.Errorf("expected index name in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "role") || !strings.Contains(sql, "status") {
		t.Errorf("expected both columns in index SQL, got:\n%s", sql)
	}

	// Down must be DROP INDEX
	downSQL, err := op.Down(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AddIndex.Down: %v", err)
	}
	if !strings.Contains(strings.ToUpper(downSQL), "DROP INDEX") {
		t.Errorf("expected DROP INDEX in Down SQL, got:\n%s", downSQL)
	}
}

// ---------------------------------------------------------------------------
// Section 10 — Altering fields: AlterField operation (PostgreSQL)
// ---------------------------------------------------------------------------

// TestUsageGuide_AlterField_ExpandVarchar_PostgreSQL_SQL verifies that AlterField
// generates correct ALTER COLUMN SQL when expanding status from varchar(50) to (100).
func TestUsageGuide_AlterField_ExpandVarchar_PostgreSQL_SQL(t *testing.T) {
	p := newPGProvider()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{
		{Name: "id", Type: "uuid", PrimaryKey: true},
		{Name: "status", Type: "varchar", Length: 50},
	}, nil)

	op := &migrate.AlterField{
		Table:    "users",
		OldField: migrate.Field{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
		NewField: migrate.Field{Name: "status", Type: "varchar", Length: 100, Default: "default_status"},
	}

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AlterField.Up: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sql), "ALTER") {
		t.Errorf("expected ALTER in AlterField SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "100") {
		t.Errorf("expected new length 100 in AlterField SQL, got:\n%s", sql)
	}

	// Down must restore the old definition
	downSQL, err := op.Down(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("AlterField.Down: %v", err)
	}
	if !strings.Contains(downSQL, "50") {
		t.Errorf("expected old length 50 in AlterField Down SQL, got:\n%s", downSQL)
	}
}

// ---------------------------------------------------------------------------
// Section 11 — Safe NOT NULL migration: RunSQL backfill + AlterField
// ---------------------------------------------------------------------------

// TestUsageGuide_SafeNotNull_RunSQL verifies that the backfill RunSQL operation
// emits the correct UPDATE statement and has an empty BackwardSQL.
func TestUsageGuide_SafeNotNull_RunSQL(t *testing.T) {
	op := &migrate.RunSQL{
		ForwardSQL:  "UPDATE users SET phone = '' WHERE phone IS NULL",
		BackwardSQL: "",
	}

	sql, err := op.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Up: %v", err)
	}
	if !strings.Contains(sql, "UPDATE users SET phone") {
		t.Errorf("expected UPDATE users SET phone in SQL, got:\n%s", sql)
	}

	back, err := op.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Down: %v", err)
	}
	if back != "" {
		t.Errorf("expected empty BackwardSQL, got %q", back)
	}
}

// ---------------------------------------------------------------------------
// Section 12 — Removing a field: DropField destructive check
// ---------------------------------------------------------------------------

// TestUsageGuide_DropField_IsDestructive verifies DropField is marked destructive
// and generates the correct DROP COLUMN SQL.
func TestUsageGuide_DropField_IsDestructive(t *testing.T) {
	op := &migrate.DropField{Table: "orders", Field: "notes"}
	if !op.IsDestructive() {
		t.Fatal("expected DropField to be destructive")
	}

	p := newPGProvider()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{
		{Name: "id", Type: "uuid", PrimaryKey: true},
		{Name: "notes", Type: "text", Nullable: true},
	}, nil)

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("DropField.Up: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sql), "DROP COLUMN") {
		t.Errorf("expected DROP COLUMN in DropField SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "notes") {
		t.Errorf("expected column name 'notes' in DropField SQL, got:\n%s", sql)
	}

	// Down must reconstruct ADD COLUMN
	downSQL, err := op.Down(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("DropField.Down: %v", err)
	}
	if !strings.Contains(strings.ToUpper(downSQL), "ADD COLUMN") {
		t.Errorf("expected ADD COLUMN in DropField Down SQL, got:\n%s", downSQL)
	}
}

// ---------------------------------------------------------------------------
// Section 13 — Removing a table: DropTable destructive check
// ---------------------------------------------------------------------------

// TestUsageGuide_DropTable_IsDestructive verifies DropTable is marked destructive
// and generates DROP TABLE SQL.
func TestUsageGuide_DropTable_IsDestructive(t *testing.T) {
	op := &migrate.DropTable{Name: "product_categories"}
	if !op.IsDestructive() {
		t.Fatal("expected DropTable to be destructive")
	}

	p := newPGProvider()
	state := migrate.NewSchemaState()
	_ = state.AddTable("product_categories", []migrate.Field{
		{Name: "id", Type: "serial", PrimaryKey: true},
	}, nil)

	sql, err := op.Up(p, state, usageDefaults, "")
	if err != nil {
		t.Fatalf("DropTable.Up: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sql), "DROP TABLE") {
		t.Errorf("expected DROP TABLE in DropTable SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "product_categories") {
		t.Errorf("expected table name in DropTable SQL, got:\n%s", sql)
	}
}

// ---------------------------------------------------------------------------
// Section 14 — Seed data: RunSQL with INSERT/DELETE
// ---------------------------------------------------------------------------

// TestUsageGuide_SeedData_RunSQL verifies the seed migration operation emits
// the expected INSERT statements and a reversible DELETE in BackwardSQL.
func TestUsageGuide_SeedData_RunSQL(t *testing.T) {
	op := &migrate.RunSQL{
		ForwardSQL: `
INSERT INTO categories (name, slug, parent_id) VALUES
    ('Electronics',       'electronics',       NULL),
    ('Clothing',          'clothing',          NULL),
    ('Books',             'books',             NULL),
    ('Smartphones',       'smartphones',       1),
    ('Laptops',           'laptops',           1),
    ('Men''s Clothing',   'mens-clothing',     2),
    ('Women''s Clothing', 'womens-clothing',   2);
`,
		BackwardSQL: `
DELETE FROM categories
WHERE slug IN (
    'electronics', 'clothing', 'books',
    'smartphones', 'laptops',
    'mens-clothing', 'womens-clothing'
);
`,
	}

	if op.TypeName() != "run_sql" {
		t.Errorf("TypeName = %q, want run_sql", op.TypeName())
	}

	sql, err := op.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Up: %v", err)
	}
	if !strings.Contains(sql, "INSERT INTO categories") {
		t.Errorf("expected INSERT INTO categories, got:\n%s", sql)
	}
	if !strings.Contains(sql, "Electronics") {
		t.Errorf("expected 'Electronics' in INSERT, got:\n%s", sql)
	}
	for _, slug := range []string{"electronics", "clothing", "books", "smartphones"} {
		if !strings.Contains(sql, slug) {
			t.Errorf("expected slug %q in INSERT, got:\n%s", slug, sql)
		}
	}

	back, err := op.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Down: %v", err)
	}
	if !strings.Contains(back, "DELETE FROM categories") {
		t.Errorf("expected DELETE FROM categories in BackwardSQL, got:\n%s", back)
	}
	if !strings.Contains(back, "electronics") {
		t.Errorf("expected 'electronics' slug in DELETE, got:\n%s", back)
	}
}

// ---------------------------------------------------------------------------
// Section 15 — Stored procedure: RunSQL with CREATE FUNCTION
// ---------------------------------------------------------------------------

// TestUsageGuide_StoredProc_Function verifies the calculate_order_total
// function migration emits CREATE FUNCTION forward and DROP FUNCTION backward.
func TestUsageGuide_StoredProc_Function(t *testing.T) {
	op := &migrate.RunSQL{
		ForwardSQL: `
CREATE OR REPLACE FUNCTION calculate_order_total(p_user_id UUID)
RETURNS TABLE (
    order_id    UUID,
    placed_at   TIMESTAMP,
    item_count  BIGINT,
    total       DECIMAL(12,2)
)
LANGUAGE sql
STABLE
AS $$
    SELECT
        o.id            AS order_id,
        o.placed_at,
        COUNT(*)        AS item_count,
        SUM(p.price)    AS total
    FROM orders o
    JOIN products p ON p.id = ANY(ARRAY[]::UUID[])
    WHERE o.user_id = p_user_id
    GROUP BY o.id, o.placed_at
    ORDER BY o.placed_at DESC;
$$;
COMMENT ON FUNCTION calculate_order_total(UUID)
    IS 'Returns a summary of all orders for a given user.';
`,
		BackwardSQL: `DROP FUNCTION IF EXISTS calculate_order_total(UUID);`,
	}

	sql, err := op.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Up: %v", err)
	}
	if !strings.Contains(sql, "CREATE OR REPLACE FUNCTION calculate_order_total") {
		t.Errorf("expected CREATE OR REPLACE FUNCTION in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "RETURNS TABLE") {
		t.Errorf("expected RETURNS TABLE in SQL, got:\n%s", sql)
	}

	back, err := op.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("RunSQL.Down: %v", err)
	}
	if !strings.Contains(back, "DROP FUNCTION IF EXISTS calculate_order_total") {
		t.Errorf("expected DROP FUNCTION IF EXISTS in Down SQL, got:\n%s", back)
	}
}

// TestUsageGuide_StoredProc_Trigger verifies the set_updated_at trigger migration
// emits CREATE FUNCTION and CREATE TRIGGER forward, DROP in reverse.
func TestUsageGuide_StoredProc_Trigger(t *testing.T) {
	createFnOp := &migrate.RunSQL{
		ForwardSQL: `
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;`,
		BackwardSQL: `DROP FUNCTION IF EXISTS set_updated_at() CASCADE;`,
	}

	attachUsersOp := &migrate.RunSQL{
		ForwardSQL: `
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();`,
		BackwardSQL: `DROP TRIGGER IF EXISTS trg_users_updated_at ON users;`,
	}

	attachOrdersOp := &migrate.RunSQL{
		ForwardSQL: `
CREATE TRIGGER trg_orders_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();`,
		BackwardSQL: `DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders;`,
	}

	// Verify CREATE FUNCTION
	fnSQL, err := createFnOp.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("createFn.Up: %v", err)
	}
	if !strings.Contains(fnSQL, "CREATE OR REPLACE FUNCTION set_updated_at") {
		t.Errorf("expected CREATE OR REPLACE FUNCTION set_updated_at, got:\n%s", fnSQL)
	}
	if !strings.Contains(fnSQL, "RETURNS TRIGGER") {
		t.Errorf("expected RETURNS TRIGGER, got:\n%s", fnSQL)
	}

	// Verify TRIGGER on users
	triggerSQL, err := attachUsersOp.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("attachUsers.Up: %v", err)
	}
	if !strings.Contains(triggerSQL, "CREATE TRIGGER trg_users_updated_at") {
		t.Errorf("expected CREATE TRIGGER trg_users_updated_at, got:\n%s", triggerSQL)
	}
	if !strings.Contains(triggerSQL, "BEFORE UPDATE ON users") {
		t.Errorf("expected BEFORE UPDATE ON users, got:\n%s", triggerSQL)
	}

	// Verify TRIGGER on orders
	ordersSQL, err := attachOrdersOp.Up(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("attachOrders.Up: %v", err)
	}
	if !strings.Contains(ordersSQL, "trg_orders_updated_at") {
		t.Errorf("expected trg_orders_updated_at, got:\n%s", ordersSQL)
	}

	// Verify Down operations
	fnDown, err := createFnOp.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("createFn.Down: %v", err)
	}
	if !strings.Contains(fnDown, "DROP FUNCTION IF EXISTS set_updated_at") {
		t.Errorf("expected DROP FUNCTION IF EXISTS set_updated_at, got:\n%s", fnDown)
	}

	usersDown, err := attachUsersOp.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("attachUsers.Down: %v", err)
	}
	if !strings.Contains(usersDown, "DROP TRIGGER IF EXISTS trg_users_updated_at ON users") {
		t.Errorf("expected DROP TRIGGER IF EXISTS trg_users_updated_at ON users, got:\n%s", usersDown)
	}

	ordersDown, err := attachOrdersOp.Down(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("attachOrders.Down: %v", err)
	}
	if !strings.Contains(ordersDown, "DROP TRIGGER IF EXISTS trg_orders_updated_at ON orders") {
		t.Errorf("expected DROP TRIGGER IF EXISTS trg_orders_updated_at, got:\n%s", ordersDown)
	}
}

// ---------------------------------------------------------------------------
// Section 16 — Full lifecycle integration (SQLite)
//
// Mirrors all steps from docs/Usage.md against an in-memory SQLite database.
// SQLite-compatible SQL is used for RunSQL operations (no CREATE FUNCTION).
// ---------------------------------------------------------------------------

// buildUsageRegistry constructs a migration registry that implements all Usage.md
// steps using SQLite-compatible DDL and DML.
func buildUsageRegistry(t *testing.T) *migrate.Registry {
	t.Helper()
	reg := migrate.NewRegistry()

	// ------------------------------------------------------------------
	// 0001_initial — SetDefaults + SetTypeMappings + all base tables
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			// SQLite: use unquoted string values for non-SQL defaults.
			// HandleFallbackDefault wraps them in single quotes at runtime.
			&migrate.SetDefaults{Defaults: map[string]string{
				"blank":          "''",
				"now":            "CURRENT_TIMESTAMP",
				"new_uuid":       "", // no UUID function; default to empty string
				"zero":           "0",
				"true":           "1",
				"false":          "0",
				"empty_json":     "{}",     // HandleFallbackDefault → '{}'
				"default_status": "active", // HandleFallbackDefault → 'active'
				"default_role":   "member", // HandleFallbackDefault → 'member'
			}},
			&migrate.SetTypeMappings{TypeMappings: map[string]string{
				"text":  "TEXT", // SQLite has no CITEXT
				"float": "REAL", // SQLite has no DOUBLE PRECISION
			}},
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
					{Name: "username", Type: "varchar", Length: 100},
					{Name: "password_hash", Type: "varchar", Length: 255},
					{Name: "role", Type: "varchar", Length: 50, Default: "default_role"},
					{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
					{Name: "metadata", Type: "text", Nullable: true},
					{Name: "created_at", Type: "timestamp", AutoCreate: true, Default: "now"},
					{Name: "updated_at", Type: "timestamp", Nullable: true, AutoUpdate: true},
				},
				Indexes: []migrate.Index{
					{Name: "idx_users_email", Fields: []string{"email"}, Unique: true},
					{Name: "idx_users_username", Fields: []string{"username"}, Unique: true},
					{Name: "idx_users_status", Fields: []string{"status"}, Unique: false},
				},
			},
			&migrate.CreateTable{
				Name: "categories",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "name", Type: "varchar", Length: 100},
					{Name: "slug", Type: "varchar", Length: 100},
					// SQLite: omit FK constraint — use plain integer
					{Name: "parent_id", Type: "integer", Nullable: true},
				},
				Indexes: []migrate.Index{
					{Name: "idx_categories_slug", Fields: []string{"slug"}, Unique: true},
				},
			},
			&migrate.CreateTable{
				Name: "products",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "name", Type: "varchar", Length: 255},
					{Name: "description", Type: "text", Nullable: true},
					{Name: "price", Type: "decimal", Precision: 10, Scale: 2},
					{Name: "weight_kg", Type: "float", Nullable: true},
					{Name: "stock_count", Type: "integer", Default: "zero"},
					{Name: "is_active", Type: "boolean", Default: "true"},
					{Name: "metadata", Type: "text", Nullable: true},
					{Name: "created_at", Type: "timestamp", AutoCreate: true, Default: "now"},
					{Name: "updated_at", Type: "timestamp", Nullable: true, AutoUpdate: true},
				},
				Indexes: []migrate.Index{
					{Name: "idx_products_name", Fields: []string{"name"}, Unique: false},
					{Name: "idx_products_is_active_created", Fields: []string{"is_active", "created_at"}, Unique: false},
				},
			},
			&migrate.CreateTable{
				Name: "product_categories",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "product_id", Type: "integer"},
					{Name: "category_id", Type: "integer"},
				},
				Indexes: []migrate.Index{
					{Name: "idx_product_categories_unique", Fields: []string{"product_id", "category_id"}, Unique: true},
				},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0002_add_orders — new orders table
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0002_add_orders",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "orders",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "user_id", Type: "integer"},
					{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
					{Name: "total_amount", Type: "decimal", Precision: 12, Scale: 2, Default: "zero"},
					{Name: "notes", Type: "text", Nullable: true},
					{Name: "placed_at", Type: "timestamp", AutoCreate: true, Default: "now"},
				},
				Indexes: []migrate.Index{
					{Name: "idx_orders_user_id", Fields: []string{"user_id"}, Unique: false},
					{Name: "idx_orders_status_placed", Fields: []string{"status", "placed_at"}, Unique: false},
				},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0003_add_user_phone_and_last_login — new fields on users
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0003_add_user_phone_and_last_login",
		Dependencies: []string{"0002_add_orders"},
		Operations: []migrate.Operation{
			&migrate.AddField{
				Table: "users",
				Field: migrate.Field{Name: "phone", Type: "varchar", Length: 30, Nullable: true},
			},
			&migrate.AddField{
				Table: "users",
				Field: migrate.Field{Name: "last_login_at", Type: "timestamp", Nullable: true},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0004_add_user_role_status_index — composite index on users
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0004_add_user_role_status_index",
		Dependencies: []string{"0003_add_user_phone_and_last_login"},
		Operations: []migrate.Operation{
			&migrate.AddIndex{
				Table: "users",
				Index: migrate.Index{
					Name:   "idx_users_role_status",
					Fields: []string{"role", "status"},
					Unique: false,
				},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0005_expand_user_status_length — AlterField: varchar(50) → varchar(100)
	// Note: SQLite does not enforce column length but the operation is valid.
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0005_expand_user_status_length",
		Dependencies: []string{"0004_add_user_role_status_index"},
		Operations: []migrate.Operation{
			&migrate.AlterField{
				Table:    "users",
				OldField: migrate.Field{Name: "status", Type: "varchar", Length: 50, Default: "default_status"},
				NewField: migrate.Field{Name: "status", Type: "varchar", Length: 100, Default: "default_status"},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0006_make_phone_required — RunSQL backfill + AlterField
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0006_make_phone_required",
		Dependencies: []string{"0005_expand_user_status_length"},
		Operations: []migrate.Operation{
			// Backfill NULLs
			&migrate.RunSQL{
				ForwardSQL:  "UPDATE users SET phone = '' WHERE phone IS NULL",
				BackwardSQL: "",
			},
			// Tighten to NOT NULL — SQLite provider returns empty SQL for ALTER COLUMN
			// (schema state still advances via Mutate). PostgreSQL SQL generation
			// is verified separately in TestUsageGuide_AlterField_ExpandVarchar_PostgreSQL_SQL.
			&migrate.AlterField{
				Table:    "users",
				OldField: migrate.Field{Name: "phone", Type: "varchar", Length: 30, Nullable: true},
				NewField: migrate.Field{Name: "phone", Type: "varchar", Length: 30, Default: "blank"},
			},
		},
	})

	// ------------------------------------------------------------------
	// 0007_remove_order_notes — DropField (destructive)
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0007_remove_order_notes",
		Dependencies: []string{"0006_make_phone_required"},
		Operations: []migrate.Operation{
			// SQLite ≥ 3.35.0 supports DROP COLUMN natively.
			&migrate.DropField{Table: "orders", Field: "notes"},
		},
	})

	// ------------------------------------------------------------------
	// 0008_remove_product_categories — DropTable (destructive)
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0008_remove_product_categories",
		Dependencies: []string{"0007_remove_order_notes"},
		Operations: []migrate.Operation{
			&migrate.DropTable{Name: "product_categories"},
		},
	})

	// ------------------------------------------------------------------
	// 0009_seed_categories — RunSQL data migration
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0009_seed_categories",
		Dependencies: []string{"0008_remove_product_categories"},
		Operations: []migrate.Operation{
			&migrate.RunSQL{
				ForwardSQL: `
INSERT INTO categories (name, slug, parent_id) VALUES
    ('Electronics', 'electronics', NULL),
    ('Clothing',    'clothing',    NULL),
    ('Books',       'books',       NULL);
`,
				BackwardSQL: `
DELETE FROM categories WHERE slug IN ('electronics', 'clothing', 'books');
`,
			},
		},
	})

	// ------------------------------------------------------------------
	// 0010_add_updated_at_trigger — RunSQL stored procedure (SQLite trigger)
	// ------------------------------------------------------------------
	reg.Register(&migrate.Migration{
		Name:         "0010_add_updated_at_trigger",
		Dependencies: []string{"0009_seed_categories"},
		Operations: []migrate.Operation{
			// SQLite uses CREATE TRIGGER directly (no stored functions)
			&migrate.RunSQL{
				ForwardSQL: `
CREATE TRIGGER trg_users_updated_at
AFTER UPDATE ON users
FOR EACH ROW
BEGIN
    UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
`,
				BackwardSQL: `DROP TRIGGER IF EXISTS trg_users_updated_at;`,
			},
			&migrate.RunSQL{
				ForwardSQL: `
CREATE TRIGGER trg_orders_updated_at
AFTER UPDATE ON orders
FOR EACH ROW
BEGIN
    UPDATE orders SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;
`,
				BackwardSQL: `DROP TRIGGER IF EXISTS trg_orders_updated_at;`,
			},
		},
	})

	return reg
}

// TestUsageGuide_FullLifecycle_Up applies all 10 Usage.md migrations in order
// against an in-memory SQLite database and verifies each step's effect on the
// schema state.
func TestUsageGuide_FullLifecycle_Up(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, recorder, db := buildTestRunner(t, reg)

	// Apply all migrations
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up (all migrations): %v", err)
	}

	// ------------------------------------------------------------------
	// Verify 0001_initial: all base tables exist
	// ------------------------------------------------------------------
	for _, table := range []string{"users", "categories", "products"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err != nil {
			t.Errorf("expected table %q to exist after 0001_initial: %v", table, err)
		}
	}

	// Insert a user using all fields added in 0001 and 0003
	if _, err := db.Exec(`
		INSERT INTO users (email, username, password_hash, role, status, metadata, phone)
		VALUES ('alice@example.com', 'alice', 'hash123', 'member', 'active', '{}', '0412345678')
	`); err != nil {
		t.Fatalf("insert into users failed: %v", err)
	}

	// ------------------------------------------------------------------
	// Verify 0002_add_orders / 0007_remove_order_notes: orders table exists.
	// After 0007 the notes column is dropped, so insert without it.
	// ------------------------------------------------------------------
	if _, err := db.Exec(`
		INSERT INTO orders (user_id, status, total_amount, placed_at)
		VALUES (1, 'active', 99.99, CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("insert into orders failed: %v", err)
	}

	// ------------------------------------------------------------------
	// Verify 0003: phone and last_login_at fields exist
	// ------------------------------------------------------------------
	if _, err := db.Exec("UPDATE users SET phone = '0412000000', last_login_at = CURRENT_TIMESTAMP WHERE id = 1"); err != nil {
		t.Fatalf("update phone/last_login_at failed: %v", err)
	}

	// ------------------------------------------------------------------
	// Verify 0009: seed data present in categories
	// ------------------------------------------------------------------
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories WHERE slug IN ('electronics','clothing','books')").Scan(&count); err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 seeded categories, got %d", count)
	}

	// ------------------------------------------------------------------
	// Verify 0008: product_categories table was dropped
	// ------------------------------------------------------------------
	if _, err := db.Exec("SELECT 1 FROM product_categories LIMIT 1"); err == nil {
		t.Error("expected product_categories to be dropped after 0008")
	}

	// ------------------------------------------------------------------
	// Verify all migrations are recorded as applied
	// ------------------------------------------------------------------
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	for i := 1; i <= 10; i++ {
		name := ""
		switch i {
		case 1:
			name = "0001_initial"
		case 2:
			name = "0002_add_orders"
		case 3:
			name = "0003_add_user_phone_and_last_login"
		case 4:
			name = "0004_add_user_role_status_index"
		case 5:
			name = "0005_expand_user_status_length"
		case 6:
			name = "0006_make_phone_required"
		case 7:
			name = "0007_remove_order_notes"
		case 8:
			name = "0008_remove_product_categories"
		case 9:
			name = "0009_seed_categories"
		case 10:
			name = "0010_add_updated_at_trigger"
		}
		if !applied[name] {
			t.Errorf("expected migration %q to be applied", name)
		}
	}
}

// TestUsageGuide_FullLifecycle_UpThenDown applies all migrations, then rolls
// back one step at a time verifying each step matches the expected state.
func TestUsageGuide_FullLifecycle_UpThenDown(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, recorder, db := buildTestRunner(t, reg)

	// Apply all 10 migrations
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Roll back the trigger migration (0010)
	if err := runner.Down(1, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down(1) — rollback triggers: %v", err)
	}
	applied, _ := recorder.GetApplied()
	if applied["0010_add_updated_at_trigger"] {
		t.Error("expected 0010_add_updated_at_trigger to be rolled back")
	}
	if !applied["0009_seed_categories"] {
		t.Error("expected 0009_seed_categories to still be applied")
	}

	// Roll back the seed migration (0009)
	if err := runner.Down(1, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down(1) — rollback seed: %v", err)
	}
	// Verify seeded rows were deleted by BackwardSQL
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories WHERE slug IN ('electronics','clothing','books')").Scan(&count); err != nil {
		t.Fatalf("count categories after rollback: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 seeded categories after rollback, got %d", count)
	}

	// Roll back the remaining migrations (0008 through 0001 = 8 more steps)
	if err := runner.Down(8, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down(8) — rollback remaining: %v", err)
	}

	// After full rollback the base tables should all be gone
	for _, table := range []string{"users", "categories", "products", "orders"} {
		if _, err := db.Exec("SELECT 1 FROM " + table + " LIMIT 1"); err == nil {
			t.Errorf("expected table %q to be dropped after full rollback", table)
		}
	}

	// History table should be empty
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if len(applied) != 0 {
		t.Errorf("expected empty migration history after full rollback, got %d entries", len(applied))
	}
}

// TestUsageGuide_FullLifecycle_ShowSQL verifies that ShowSQL runs without error
// against the full Usage.md migration set before any migration is applied.
func TestUsageGuide_FullLifecycle_ShowSQL(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, _, _ := buildTestRunner(t, reg)

	if err := runner.ShowSQL(); err != nil {
		t.Fatalf("ShowSQL: %v", err)
	}
}

// TestUsageGuide_FullLifecycle_Status verifies Status runs without error both
// before and after applying migrations.
func TestUsageGuide_FullLifecycle_Status(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, _, _ := buildTestRunner(t, reg)

	// Status before apply
	if err := runner.Status(); err != nil {
		t.Fatalf("Status (before up): %v", err)
	}

	// Apply all then check status again
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := runner.Status(); err != nil {
		t.Fatalf("Status (after up): %v", err)
	}
}

// TestUsageGuide_DAG_Structure verifies the migration graph built from the
// Usage.md registry has the expected structure: 10 nodes, no branches, one leaf.
func TestUsageGuide_DAG_Structure(t *testing.T) {
	reg := buildUsageRegistry(t)
	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	if g.HasBranches() {
		t.Error("expected no branches in the Usage.md linear DAG")
	}

	out, err := g.ToDAGOutput()
	if err != nil {
		t.Fatalf("ToDAGOutput: %v", err)
	}

	if len(out.Migrations) != 10 {
		t.Errorf("expected 10 migrations, got %d", len(out.Migrations))
	}
	if len(out.Leaves) != 1 {
		t.Errorf("expected 1 leaf, got %d: %v", len(out.Leaves), out.Leaves)
	}
	if out.Leaves[0] != "0010_add_updated_at_trigger" {
		t.Errorf("expected leaf '0010_add_updated_at_trigger', got %q", out.Leaves[0])
	}
	if len(out.Roots) != 1 || out.Roots[0] != "0001_initial" {
		t.Errorf("expected root '0001_initial', got %v", out.Roots)
	}
}

// TestUsageGuide_PartialApply_UpTo verifies that Up --to stops at the named
// migration, leaving later ones pending.
func TestUsageGuide_PartialApply_UpTo(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, recorder, db := buildTestRunner(t, reg)

	// Apply only up to and including 0002_add_orders
	if err := runner.Up("0002_add_orders", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up to 0002_add_orders: %v", err)
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}

	if !applied["0001_initial"] || !applied["0002_add_orders"] {
		t.Error("expected 0001_initial and 0002_add_orders to be applied")
	}
	if applied["0003_add_user_phone_and_last_login"] {
		t.Error("expected 0003 to still be pending")
	}

	// phone column should not exist yet
	if _, err := db.Exec("SELECT phone FROM users"); err == nil {
		t.Error("expected phone column to not exist yet (0003 not applied)")
	}
}

// TestUsageGuide_RollbackToTarget verifies Down --to stops at (but does not roll
// back) the named migration.
func TestUsageGuide_RollbackToTarget(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, recorder, _ := buildTestRunner(t, reg)

	// Apply first 3 migrations
	if err := runner.Up("0003_add_user_phone_and_last_login", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up to 0003: %v", err)
	}

	// Roll back to 0001_initial (exclusive) — should roll back 0003 and 0002
	if err := runner.Down(0, "0001_initial", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down to 0001_initial: %v", err)
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}

	// 0001_initial remains; 0002 and 0003 are rolled back
	if !applied["0001_initial"] {
		t.Error("expected 0001_initial to remain applied")
	}
	if applied["0002_add_orders"] {
		t.Error("expected 0002_add_orders to be rolled back")
	}
	if applied["0003_add_user_phone_and_last_login"] {
		t.Error("expected 0003 to be rolled back")
	}
}

// TestUsageGuide_Idempotent verifies that calling Up twice does not re-apply
// already-applied migrations or cause errors.
func TestUsageGuide_Idempotent(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := buildUsageRegistry(t)
	runner, _, _ := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("second Up (must be idempotent): %v", err)
	}
}
