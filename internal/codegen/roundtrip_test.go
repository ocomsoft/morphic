package codegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/yaml"
	"github.com/ocomsoft/morphic/migrate"
)

// TestStarlark_RoundTrip_CreateTable verifies that a generated .star file can be
// loaded back by the Starlark loader and produces the expected operations.
func TestStarlark_RoundTrip_CreateTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	nullable := true
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeTableAdded,
				TableName: "users",
				NewValue: yaml.Table{
					Name: "users",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid", Nullable: &nullable},
						{Name: "email", Type: "varchar", Length: 255, Nullable: &nullable},
					},
					Indexes: []yaml.Index{
						{Name: "users_email_idx", Fields: []string{"email"}, Unique: true},
					},
				},
			},
		},
	}

	src, err := gen.GenerateMigration("0001_initial", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}

	m := migrations[0]
	if m.Name != "0001_initial" {
		t.Errorf("expected name '0001_initial', got %q", m.Name)
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}

	ct, ok := m.Operations[0].(*migrate.CreateTable)
	if !ok {
		t.Fatalf("expected CreateTable, got %T", m.Operations[0])
	}
	if ct.Name != "users" {
		t.Errorf("expected table 'users', got %q", ct.Name)
	}
	if len(ct.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(ct.Fields))
	}
	if !ct.Fields[0].PrimaryKey {
		t.Error("expected first field to be primary key")
	}
	if ct.Fields[0].Default != "new_uuid" {
		t.Errorf("expected default 'new_uuid', got %q", ct.Fields[0].Default)
	}
	if ct.Fields[1].Length != 255 {
		t.Errorf("expected length 255, got %d", ct.Fields[1].Length)
	}
	if len(ct.Indexes) != 1 {
		t.Fatalf("expected 1 index, got %d", len(ct.Indexes))
	}
	if !ct.Indexes[0].Unique {
		t.Error("expected index to be unique")
	}
}

// TestStarlark_RoundTrip_SetDefaults verifies set_defaults round-trips correctly.
func TestStarlark_RoundTrip_SetDefaults(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:     yaml.ChangeTypeDefaultsModified,
				NewValue: map[string]string{"new_uuid": "gen_random_uuid()", "now": "now()"},
			},
		},
	}

	src, err := gen.GenerateMigration("0001_defaults", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_defaults.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	m := reg.All()[0]
	sd, ok := m.Operations[0].(*migrate.SetDefaults)
	if !ok {
		t.Fatalf("expected SetDefaults, got %T", m.Operations[0])
	}
	if sd.Defaults["new_uuid"] != "gen_random_uuid()" {
		t.Errorf("expected 'gen_random_uuid()', got %q", sd.Defaults["new_uuid"])
	}
	if sd.Defaults["now"] != "now()" {
		t.Errorf("expected 'now()', got %q", sd.Defaults["now"])
	}
}

// TestStarlark_RoundTrip_AddField verifies add_field round-trips correctly.
func TestStarlark_RoundTrip_AddField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	nullable := true
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeFieldAdded,
				TableName: "users",
				NewValue:  yaml.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: &nullable},
			},
		},
	}

	src, err := gen.GenerateMigration("0002_add_phone", []string{"0001_initial"}, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0002_add_phone.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	m := reg.All()[0]
	if len(m.Dependencies) != 1 || m.Dependencies[0] != "0001_initial" {
		t.Errorf("expected deps [0001_initial], got %v", m.Dependencies)
	}
	af, ok := m.Operations[0].(*migrate.AddField)
	if !ok {
		t.Fatalf("expected AddField, got %T", m.Operations[0])
	}
	if af.Table != "users" {
		t.Errorf("expected table 'users', got %q", af.Table)
	}
	if af.Field.Name != "phone" || af.Field.Length != 20 {
		t.Errorf("expected phone/20, got %s/%d", af.Field.Name, af.Field.Length)
	}
}

// TestE2E_Starlark_RealisticMigration tests the full pipeline with a realistic
// AirRadiators-style schema: SetDefaults + CreateTable with FK + indexes.
func TestE2E_Starlark_RealisticMigration(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	nullable := true
	notNull := false

	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			// SetDefaults
			{
				Type: yaml.ChangeTypeDefaultsModified,
				NewValue: map[string]string{
					"new_uuid": "gen_random_uuid()",
					"now":      "now()",
				},
			},
			// SetTypeMappings
			{
				Type: yaml.ChangeTypeTypeMappingsModified,
				NewValue: map[string]string{
					"text": "varchar(max)",
				},
			},
			// CreateTable with fields and indexes
			{
				Type:      yaml.ChangeTypeTableAdded,
				TableName: "tmc_entry",
				NewValue: yaml.Table{
					Name: "tmc_entry",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid", Nullable: &notNull},
						{Name: "code", Type: "varchar", Length: 50, Nullable: &notNull},
						{Name: "description", Type: "text", Nullable: &nullable},
						{Name: "created_date", Type: "timestamp", Default: "now", Nullable: &notNull},
						{Name: "created_user_id", Type: "foreign_key", Nullable: &nullable,
							ForeignKey: &yaml.ForeignKey{Table: "auth_user", OnDelete: "PROTECT"}},
						{Name: "modified_date", Type: "timestamp", Default: "now", AutoUpdate: true, Nullable: &notNull},
						{Name: "is_active", Type: "boolean", Default: "true", Nullable: &notNull},
					},
					Indexes: []yaml.Index{
						{Name: "tmc_entry_code_idx", Fields: []string{"code"}, Unique: true},
						{Name: "tmc_entry_active_idx", Fields: []string{"is_active"},
							Where: "is_active = true"},
					},
				},
			},
			// AddField
			{
				Type:      yaml.ChangeTypeFieldAdded,
				TableName: "tmc_entry",
				NewValue:  yaml.Field{Name: "image_id", Type: "uuid", Nullable: &nullable},
			},
			// AddForeignKey
			{
				Type:      yaml.ChangeTypeForeignKeyAdded,
				TableName: "tmc_entry",
				FieldName: "created_user_id",
				NewValue: yaml.Field{
					Name: "created_user_id",
					Type: "foreign_key",
					ForeignKey: &yaml.ForeignKey{
						Table:    "auth_user",
						OnDelete: "PROTECT",
					},
				},
			},
		},
	}

	src, err := gen.GenerateMigration("0001_initial", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}

	// Write and load
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v\nSource:\n%s", err, src)
	}

	m := reg.All()[0]
	if m.Name != "0001_initial" {
		t.Errorf("expected name '0001_initial', got %q", m.Name)
	}

	// Verify all 5 operations round-tripped
	if len(m.Operations) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(m.Operations))
	}

	// Op 0: SetDefaults
	sd, ok := m.Operations[0].(*migrate.SetDefaults)
	if !ok {
		t.Fatalf("op[0]: expected SetDefaults, got %T", m.Operations[0])
	}
	if sd.Defaults["new_uuid"] != "gen_random_uuid()" {
		t.Errorf("expected new_uuid default, got %q", sd.Defaults["new_uuid"])
	}

	// Op 1: SetTypeMappings
	stm, ok := m.Operations[1].(*migrate.SetTypeMappings)
	if !ok {
		t.Fatalf("op[1]: expected SetTypeMappings, got %T", m.Operations[1])
	}
	if stm.TypeMappings["text"] != "varchar(max)" {
		t.Errorf("expected text mapping, got %q", stm.TypeMappings["text"])
	}

	// Op 2: CreateTable
	ct, ok := m.Operations[2].(*migrate.CreateTable)
	if !ok {
		t.Fatalf("op[2]: expected CreateTable, got %T", m.Operations[2])
	}
	if ct.Name != "tmc_entry" {
		t.Errorf("expected table 'tmc_entry', got %q", ct.Name)
	}
	if len(ct.Fields) != 7 {
		t.Errorf("expected 7 fields, got %d", len(ct.Fields))
	}
	if len(ct.Indexes) != 2 {
		t.Errorf("expected 2 indexes, got %d", len(ct.Indexes))
	}

	// Verify FK on field
	var fkField *migrate.Field
	for i := range ct.Fields {
		if ct.Fields[i].Name == "created_user_id" {
			fkField = &ct.Fields[i]
			break
		}
	}
	if fkField == nil {
		t.Fatal("expected to find created_user_id field")
	}
	if fkField.ForeignKey == nil {
		t.Fatal("expected FK on created_user_id")
	}
	if fkField.ForeignKey.Table != "auth_user" {
		t.Errorf("expected FK table 'auth_user', got %q", fkField.ForeignKey.Table)
	}

	// Verify AutoUpdate on modified_date
	var modField *migrate.Field
	for i := range ct.Fields {
		if ct.Fields[i].Name == "modified_date" {
			modField = &ct.Fields[i]
			break
		}
	}
	if modField == nil {
		t.Fatal("expected to find modified_date field")
	}
	if !modField.AutoUpdate {
		t.Error("expected auto_update=true on modified_date")
	}

	// Verify partial index with Where clause
	if ct.Indexes[1].Where != "is_active = true" {
		t.Errorf("expected where clause, got %q", ct.Indexes[1].Where)
	}

	// Op 3: AddField
	af, ok := m.Operations[3].(*migrate.AddField)
	if !ok {
		t.Fatalf("op[3]: expected AddField, got %T", m.Operations[3])
	}
	if af.Field.Name != "image_id" {
		t.Errorf("expected image_id, got %q", af.Field.Name)
	}

	// Op 4: AddForeignKey
	afk, ok := m.Operations[4].(*migrate.AddForeignKey)
	if !ok {
		t.Fatalf("op[4]: expected AddForeignKey, got %T", m.Operations[4])
	}
	if afk.ReferencedTable != "auth_user" {
		t.Errorf("expected referenced_table 'auth_user', got %q", afk.ReferencedTable)
	}
}

// TestE2E_Starlark_MultiMigration tests loading multiple .star files that form a chain.
func TestE2E_Starlark_MultiMigration(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	nullable := true
	notNull := false

	// Migration 1: create table
	diff1 := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeTableAdded,
				TableName: "products",
				NewValue: yaml.Table{
					Name: "products",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Nullable: &notNull},
						{Name: "name", Type: "varchar", Length: 200, Nullable: &notNull},
					},
				},
			},
		},
	}
	src1, err := gen.GenerateMigration("0001_initial", nil, diff1, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Migration 2: add field
	diff2 := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeFieldAdded,
				TableName: "products",
				NewValue:  yaml.Field{Name: "price", Type: "decimal", Precision: 10, Scale: 2, Nullable: &nullable},
			},
		},
	}
	src2, err := gen.GenerateMigration("0002_add_price", []string{"0001_initial"}, diff2, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0002_add_price.star"), []byte(src2), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}

	// Verify dependency chain
	m2 := migrations[1]
	if len(m2.Dependencies) != 1 || m2.Dependencies[0] != "0001_initial" {
		t.Errorf("expected dep on 0001_initial, got %v", m2.Dependencies)
	}

	af, ok := m2.Operations[0].(*migrate.AddField)
	if !ok {
		t.Fatalf("expected AddField, got %T", m2.Operations[0])
	}
	if af.Field.Precision != 10 || af.Field.Scale != 2 {
		t.Errorf("expected precision=10 scale=2, got %d/%d", af.Field.Precision, af.Field.Scale)
	}
}
