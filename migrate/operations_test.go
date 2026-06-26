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

package migrate_test

import (
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/providers/postgresql"
	"github.com/ocomsoft/morphic/internal/providers/sqlite"
	"github.com/ocomsoft/morphic/migrate"
)

func TestCreateTable_Up(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	op := &migrate.CreateTable{
		Name: "users",
		Fields: []migrate.Field{
			{Name: "id", Type: "integer", PrimaryKey: true},
			{Name: "email", Type: "varchar", Length: 255},
		},
	}
	sql, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if _, ok := state.Tables["users"]; !ok {
		t.Fatal("expected 'users' in state after Mutate")
	}
}

func TestCreateTable_Down(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	op := &migrate.CreateTable{Name: "users", Fields: []migrate.Field{{Name: "id", Type: "integer"}}}
	sql, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if sql == "" {
		t.Fatal("expected non-empty down SQL")
	}
}

func TestAddField_UpDown(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	op := &migrate.AddField{
		Table: "users",
		Field: migrate.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: true},
	}
	upSQL, err := op.Up(p, state, nil, "")
	if err != nil || upSQL == "" {
		t.Fatalf("Up: err=%v sql=%q", err, upSQL)
	}
	downSQL, err := op.Down(p, state, nil, "")
	if err != nil || downSQL == "" {
		t.Fatalf("Down: err=%v sql=%q", err, downSQL)
	}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if len(state.Tables["users"].Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(state.Tables["users"].Fields))
	}
}

func TestDropTable_IsDestructive(t *testing.T) {
	op := &migrate.DropTable{Name: "users"}
	if !op.IsDestructive() {
		t.Fatal("expected DropTable to be destructive")
	}
}

func TestDropField_IsDestructive(t *testing.T) {
	op := &migrate.DropField{Table: "users", Field: "email"}
	if !op.IsDestructive() {
		t.Fatal("expected DropField to be destructive")
	}
}

func TestCreateTable_IsNotDestructive(t *testing.T) {
	op := &migrate.CreateTable{Name: "t"}
	if op.IsDestructive() {
		t.Fatal("CreateTable should not be destructive")
	}
}

func TestRunSQL_UpDown(t *testing.T) {
	op := &migrate.RunSQL{
		ForwardSQL:  "UPDATE posts SET slug = 'x'",
		BackwardSQL: "UPDATE posts SET slug = NULL",
	}
	sql, _ := op.Up(nil, nil, nil, "")
	if sql != "UPDATE posts SET slug = 'x'" {
		t.Fatalf("expected forward SQL, got %q", sql)
	}
	back, _ := op.Down(nil, nil, nil, "")
	if back != "UPDATE posts SET slug = NULL" {
		t.Fatalf("expected backward SQL, got %q", back)
	}
	if op.Mutate(migrate.NewSchemaState()) != nil {
		t.Fatal("RunSQL.Mutate should not error")
	}
}

func TestRunSQL_TypeName(t *testing.T) {
	op := &migrate.RunSQL{}
	if op.TypeName() != "run_sql" {
		t.Fatalf("expected 'run_sql', got %q", op.TypeName())
	}
}

func TestRunSQL_Describe(t *testing.T) {
	short := &migrate.RunSQL{ForwardSQL: "SELECT 1"}
	if got := short.Describe(); got != "Run SQL: SELECT 1" {
		t.Errorf("short SQL: expected %q, got %q", "Run SQL: SELECT 1", got)
	}

	long := &migrate.RunSQL{ForwardSQL: strings.Repeat("x", 100)}
	got := long.Describe()
	if !strings.HasPrefix(got, "Run SQL: ") {
		t.Errorf("long SQL should start with 'Run SQL: ', got %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("long SQL should be truncated with …, got %q", got)
	}
	if len(got) > 100 {
		t.Errorf("long SQL describe should be truncated, got length %d", len(got))
	}
}

func TestDropField_Down_ReconstructsFromState(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{
		{Name: "id", Type: "integer"},
		{Name: "email", Type: "varchar", Length: 255},
	}, nil)
	op := &migrate.DropField{Table: "users", Field: "email"}
	sql, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if sql == "" {
		t.Fatal("expected down SQL for DropField")
	}
}

func TestAllTypenames(t *testing.T) {
	cases := []struct {
		op       migrate.Operation
		expected string
	}{
		{&migrate.CreateTable{Name: "t"}, "create_table"},
		{&migrate.DropTable{Name: "t"}, "drop_table"},
		{&migrate.RenameTable{OldName: "a", NewName: "b"}, "rename_table"},
		{&migrate.AddField{Table: "t", Field: migrate.Field{Name: "f", Type: "text"}}, "add_field"},
		{&migrate.DropField{Table: "t", Field: "f"}, "drop_field"},
		{&migrate.AlterField{Table: "t"}, "alter_field"},
		{&migrate.RenameField{Table: "t", OldName: "a", NewName: "b"}, "rename_field"},
		{&migrate.AddIndex{Table: "t", Index: migrate.Index{Name: "i", Fields: []string{"f"}}}, "add_index"},
		{&migrate.DropIndex{Table: "t", Index: "i"}, "drop_index"},
		{&migrate.RunSQL{}, "run_sql"},
		{&migrate.AddForeignKey{Table: "t", FieldName: "fk_id", ReferencedTable: "other", ConstraintName: "fk_t_other"}, "add_foreign_key"},
		{&migrate.DropForeignKey{Table: "t", ConstraintName: "fk_t_other"}, "drop_foreign_key"},
	}
	for _, tc := range cases {
		if tc.op.TypeName() != tc.expected {
			t.Errorf("%T: expected TypeName %q, got %q", tc.op, tc.expected, tc.op.TypeName())
		}
	}
}

func TestDropTable_SchemaOnly_NoSQL(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	// Pre-populate state so Down() can reconstruct the CREATE TABLE.
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	op := &migrate.DropTable{Name: "users", SchemaOnly: true}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL != "" {
		t.Errorf("expected empty Up SQL with SchemaOnly, got %q", upSQL)
	}

	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if downSQL != "" {
		t.Errorf("expected empty Down SQL with SchemaOnly, got %q", downSQL)
	}

	// Mutate must still advance schema state.
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if _, exists := state.Tables["users"]; exists {
		t.Error("expected table 'users' to be removed from state after Mutate")
	}
}

func TestDropField_SchemaOnly_NoSQL(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{
		{Name: "id", Type: "integer", PrimaryKey: true},
		{Name: "phone", Type: "varchar"},
	}, nil)

	op := &migrate.DropField{Table: "users", Field: "phone", SchemaOnly: true}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL != "" {
		t.Errorf("expected empty Up SQL with SchemaOnly, got %q", upSQL)
	}

	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if downSQL != "" {
		t.Errorf("expected empty Down SQL with SchemaOnly, got %q", downSQL)
	}

	// Mutate must still advance schema state.
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	ts, exists := state.Tables["users"]
	if !exists {
		t.Fatal("expected table 'users' to still exist in state")
	}
	for _, f := range ts.Fields {
		if f.Name == "phone" {
			t.Error("expected field 'phone' to be removed from state after Mutate")
		}
	}
}

func TestCreateTable_SchemaOnly_NoSQL(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()

	op := &migrate.CreateTable{
		Name:       "users",
		Fields:     []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}},
		SchemaOnly: true,
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL != "" {
		t.Errorf("expected empty Up SQL with SchemaOnly, got %q", upSQL)
	}

	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if downSQL != "" {
		t.Errorf("expected empty Down SQL with SchemaOnly, got %q", downSQL)
	}

	// Mutate must still advance schema state.
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if _, exists := state.Tables["users"]; !exists {
		t.Error("expected table 'users' to be present in state after Mutate")
	}
}

func TestAddField_SchemaOnly_NoSQL(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	op := &migrate.AddField{
		Table:      "users",
		Field:      migrate.Field{Name: "email", Type: "varchar"},
		SchemaOnly: true,
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL != "" {
		t.Errorf("expected empty Up SQL with SchemaOnly, got %q", upSQL)
	}

	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if downSQL != "" {
		t.Errorf("expected empty Down SQL with SchemaOnly, got %q", downSQL)
	}

	// Mutate must still advance schema state.
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	ts, exists := state.Tables["users"]
	if !exists {
		t.Fatal("expected table 'users' in state")
	}
	found := false
	for _, f := range ts.Fields {
		if f.Name == "email" {
			found = true
		}
	}
	if !found {
		t.Error("expected field 'email' to be present in state after Mutate")
	}
}

// TestSetDefaults_Mutate verifies that SetDefaults.Mutate updates state.Defaults.
func TestSetDefaults_Mutate(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetDefaults{
		Defaults: map[string]string{
			"uuid": "uuid_generate_v4()",
			"now":  "CURRENT_TIMESTAMP",
		},
	}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if state.Defaults["uuid"] != "uuid_generate_v4()" {
		t.Errorf("expected uuid default, got %q", state.Defaults["uuid"])
	}
	if state.Defaults["now"] != "CURRENT_TIMESTAMP" {
		t.Errorf("expected now default, got %q", state.Defaults["now"])
	}
}

// TestSetDefaults_UpDown verifies that SetDefaults.Up and Down return empty SQL.
func TestSetDefaults_UpDown(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	op := &migrate.SetDefaults{Defaults: map[string]string{"uuid": "uuid_generate_v4()"}}
	upSQL, err := op.Up(p, state, nil, "")
	if err != nil || upSQL != "" {
		t.Errorf("SetDefaults.Up should return empty SQL, got %q err=%v", upSQL, err)
	}
	downSQL, err := op.Down(p, state, nil, "")
	if err != nil || downSQL != "" {
		t.Errorf("SetDefaults.Down should return empty SQL, got %q err=%v", downSQL, err)
	}
}

// TestSetTypeMappings_Mutate verifies that SetTypeMappings.Mutate updates state.TypeMappings.
func TestSetTypeMappings_Mutate(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetTypeMappings{
		TypeMappings: map[string]string{"float": "DOUBLE PRECISION", "text": "NVARCHAR(MAX)"},
	}
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate returned error: %v", err)
	}
	if state.TypeMappings["float"] != "DOUBLE PRECISION" {
		t.Errorf("expected DOUBLE PRECISION, got %q", state.TypeMappings["float"])
	}
	if state.TypeMappings["text"] != "NVARCHAR(MAX)" {
		t.Errorf("expected NVARCHAR(MAX), got %q", state.TypeMappings["text"])
	}
}

// TestSetTypeMappings_UpDown verifies that SetTypeMappings.Up and Down return empty SQL.
func TestSetTypeMappings_UpDown(t *testing.T) {
	state := migrate.NewSchemaState()
	op := &migrate.SetTypeMappings{TypeMappings: map[string]string{"float": "DOUBLE PRECISION"}}
	upSQL, err := op.Up(nil, state, nil, "")
	if err != nil || upSQL != "" {
		t.Errorf("SetTypeMappings.Up should return empty SQL, got %q err=%v", upSQL, err)
	}
	downSQL, err := op.Down(nil, state, nil, "")
	if err != nil || downSQL != "" {
		t.Errorf("SetTypeMappings.Down should return empty SQL, got %q err=%v", downSQL, err)
	}
}

// TestSetTypeMappings_Metadata verifies TypeName, TableName, IsDestructive, Describe.
func TestSetTypeMappings_Metadata(t *testing.T) {
	op := &migrate.SetTypeMappings{TypeMappings: map[string]string{}}
	if op.TypeName() != "set_type_mappings" {
		t.Errorf("TypeName = %q, want set_type_mappings", op.TypeName())
	}
	if op.TableName() != "" {
		t.Errorf("TableName = %q, want empty", op.TableName())
	}
	if op.IsDestructive() {
		t.Error("IsDestructive should be false")
	}
	if op.Describe() == "" {
		t.Error("Describe should be non-empty")
	}
}

// TestDropTable_Down_ResolvesDefaults verifies that DropTable.Down resolves symbolic
// default references (e.g. "new_uuid") to SQL expressions using the defaults map when
// reconstructing the CREATE TABLE SQL. Regression test for the bug where DropTable.Down
// passed fields directly without calling resolveFieldDefault, causing literal default
// values like "new_uuid" to be emitted instead of SQL expressions like "gen_random_uuid()".
func TestDropTable_Down_ResolvesDefaults(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	defaults := map[string]string{
		"new_uuid": "gen_random_uuid()",
	}
	// Pre-populate state with the table that was dropped (as it would exist before the drop)
	err := state.AddTable("items", []migrate.Field{
		{Name: "id", Type: "uuid", Default: "new_uuid", PrimaryKey: true},
	}, nil)
	if err != nil {
		t.Fatalf("AddTable: %v", err)
	}
	op := &migrate.DropTable{Name: "items"}
	sqlStr, err := op.Down(p, state, defaults, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if strings.Contains(sqlStr, `'new_uuid'`) || strings.Contains(sqlStr, `new_uuid`) {
		t.Errorf("raw symbolic default 'new_uuid' should be resolved in SQL, got:\n%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "gen_random_uuid()") {
		t.Errorf("expected resolved default 'gen_random_uuid()' in SQL, got:\n%s", sqlStr)
	}
}

// TestCreateTable_Up_ResolvesDefaults verifies that CreateTable.Up resolves symbolic
// default references (e.g. "uuid") to SQL expressions using the defaults map.
func TestCreateTable_Up_ResolvesDefaults(t *testing.T) {
	p := sqlite.New()
	state := migrate.NewSchemaState()
	defaults := map[string]string{
		"uuid": "uuid_generate_v4()",
	}
	op := &migrate.CreateTable{
		Name: "items",
		Fields: []migrate.Field{
			{Name: "id", Type: "uuid", Default: "uuid", PrimaryKey: true},
		},
	}
	sqlStr, err := op.Up(p, state, defaults, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.Contains(sqlStr, "uuid_generate_v4()") {
		t.Errorf("expected resolved default 'uuid_generate_v4()' in SQL, got:\n%s", sqlStr)
	}
	if strings.Contains(sqlStr, `'uuid'`) {
		t.Errorf("raw default 'uuid' should be resolved, got:\n%s", sqlStr)
	}
}

func TestAddForeignKey_UpDown(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	op := &migrate.AddForeignKey{
		Table:           "orders",
		FieldName:       "user_id",
		ConstraintName:  "fk_orders_user_id",
		ReferencedTable: "users",
		OnDelete:        "CASCADE",
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL == "" {
		t.Fatal("expected non-empty Up SQL")
	}
	if !strings.Contains(upSQL, "FOREIGN KEY") {
		t.Errorf("expected FOREIGN KEY in SQL, got: %s", upSQL)
	}

	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if len(state.Tables["orders"].ForeignKeys) != 1 {
		t.Fatal("expected FK in state after Mutate")
	}

	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if downSQL == "" {
		t.Fatal("expected non-empty Down SQL")
	}
	if !strings.Contains(downSQL, "DROP CONSTRAINT") {
		t.Errorf("expected DROP CONSTRAINT in Down SQL, got: %s", downSQL)
	}
}

func TestDropForeignKey_UpDown(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	fk := migrate.ForeignKeyConstraint{
		Name: "fk_orders_user_id", FieldName: "user_id",
		ReferencedTable: "users", OnDelete: "CASCADE",
	}
	_ = state.AddForeignKey("orders", fk)

	op := &migrate.DropForeignKey{
		Table:          "orders",
		ConstraintName: "fk_orders_user_id",
	}

	// Up: DROP CONSTRAINT
	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if upSQL == "" {
		t.Fatal("expected non-empty Up SQL")
	}

	// Down: reads state to reconstruct ADD CONSTRAINT (state still has FK before Mutate)
	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if !strings.Contains(downSQL, "FOREIGN KEY") {
		t.Errorf("expected FOREIGN KEY in Down SQL, got: %s", downSQL)
	}

	// Mutate removes FK from state
	if err := op.Mutate(state); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if len(state.Tables["orders"].ForeignKeys) != 0 {
		t.Fatal("expected 0 FKs after Mutate")
	}
}

func TestAddForeignKey_OnUpdate(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	op := &migrate.AddForeignKey{
		Table:           "orders",
		FieldName:       "user_id",
		ConstraintName:  "fk_orders_user_id",
		ReferencedTable: "users",
		OnDelete:        "CASCADE",
		OnUpdate:        "SET NULL",
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.Contains(upSQL, "ON DELETE CASCADE") {
		t.Errorf("expected ON DELETE CASCADE in SQL, got: %s", upSQL)
	}
	if !strings.Contains(upSQL, "ON UPDATE SET NULL") {
		t.Errorf("expected ON UPDATE SET NULL in SQL, got: %s", upSQL)
	}
}

func TestAddForeignKey_ConstraintNameUsed(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	customName := "my_custom_constraint"
	op := &migrate.AddForeignKey{
		Table:           "orders",
		FieldName:       "user_id",
		ConstraintName:  customName,
		ReferencedTable: "users",
		OnDelete:        "CASCADE",
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.Contains(upSQL, `"my_custom_constraint"`) {
		t.Errorf("expected custom constraint name in SQL, got: %s", upSQL)
	}
}

func TestAddForeignKey_OnUpdateNormalization(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	_ = state.AddTable("users", []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}}, nil)

	// Test Django-style SET_NULL normalization for OnUpdate
	op := &migrate.AddForeignKey{
		Table:           "orders",
		FieldName:       "user_id",
		ConstraintName:  "fk_orders_user_id",
		ReferencedTable: "users",
		OnDelete:        "CASCADE",
		OnUpdate:        "SET_NULL",
	}

	upSQL, err := op.Up(p, state, nil, "")
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.Contains(upSQL, "ON UPDATE SET NULL") {
		t.Errorf("expected normalized ON UPDATE SET NULL in SQL, got: %s", upSQL)
	}
}

func TestDropForeignKey_Down_WithOnUpdate(t *testing.T) {
	p := postgresql.New()
	state := migrate.NewSchemaState()
	_ = state.AddTable("orders", []migrate.Field{{Name: "id", Type: "integer"}}, nil)
	fk := migrate.ForeignKeyConstraint{
		Name: "fk_orders_user_id", FieldName: "user_id",
		ReferencedTable: "users", OnDelete: "CASCADE", OnUpdate: "SET NULL",
	}
	_ = state.AddForeignKey("orders", fk)

	op := &migrate.DropForeignKey{
		Table:          "orders",
		ConstraintName: "fk_orders_user_id",
	}

	// Down should reconstruct the FK with both OnDelete and OnUpdate
	downSQL, err := op.Down(p, state, nil, "")
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if !strings.Contains(downSQL, "ON DELETE CASCADE") {
		t.Errorf("expected ON DELETE CASCADE in Down SQL, got: %s", downSQL)
	}
	if !strings.Contains(downSQL, "ON UPDATE SET NULL") {
		t.Errorf("expected ON UPDATE SET NULL in Down SQL, got: %s", downSQL)
	}
	// Down should use the stored constraint name
	if !strings.Contains(downSQL, `"fk_orders_user_id"`) {
		t.Errorf("expected stored constraint name fk_orders_user_id in Down SQL, got: %s", downSQL)
	}
}
