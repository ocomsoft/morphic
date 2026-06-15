package codegen_test

import (
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/yaml"
)

func boolPtr(b bool) *bool { return &b }

func TestStarlarkGenerator_CreateTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeTableAdded,
				TableName: "users",
				NewValue: yaml.Table{
					Name: "users",
					Fields: []yaml.Field{
						{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid", Nullable: boolPtr(false)},
						{Name: "email", Type: "varchar", Length: 255, Nullable: boolPtr(true)},
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

	assertContains(t, src, `migration(`)
	assertContains(t, src, `name = "0001_initial"`)
	assertContains(t, src, `create_table(`)
	assertContains(t, src, `name = "users"`)
	assertContains(t, src, `field("id", "uuid"`)
	assertContains(t, src, `primary_key=True`)
	assertContains(t, src, `default="new_uuid"`)
	assertContains(t, src, `field("email", "varchar"`)
	assertContains(t, src, `nullable=True`)
	assertContains(t, src, `length=255`)
	assertContains(t, src, `index("users_email_idx"`)
	assertContains(t, src, `unique=True`)
}

func TestStarlarkGenerator_AddField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeFieldAdded,
				TableName: "users",
				NewValue:  yaml.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: boolPtr(true)},
			},
		},
	}

	src, err := gen.GenerateMigration("0002_add_phone", []string{"0001_initial"}, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}

	assertContains(t, src, `add_field("users"`)
	assertContains(t, src, `field("phone", "varchar"`)
	assertContains(t, src, `"0001_initial"`)
}

func TestStarlarkGenerator_DropTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRemoved, TableName: "old_table"},
		},
	}

	src, err := gen.GenerateMigration("0003_drop", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `drop_table(name="old_table")`)
}

func TestStarlarkGenerator_DropTable_SchemaOnly(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRemoved, TableName: "old_table"},
		},
	}
	decisions := map[int]yaml.PromptResponse{0: yaml.PromptOmit}

	src, err := gen.GenerateMigration("0003_drop", nil, diff, nil, nil, decisions)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `schema_only=True`)
}

func TestStarlarkGenerator_DropField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeFieldRemoved, TableName: "users", FieldName: "legacy_col"},
		},
	}

	src, err := gen.GenerateMigration("0004_drop_col", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `drop_field(table="users", field_name="legacy_col")`)
}

func TestStarlarkGenerator_AlterField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	oldSchema := &yaml.Schema{Tables: []yaml.Table{
		{Name: "users", Fields: []yaml.Field{
			{Name: "email", Type: "varchar", Length: 100, Nullable: boolPtr(false)},
		}},
	}}
	newSchema := &yaml.Schema{Tables: []yaml.Table{
		{Name: "users", Fields: []yaml.Field{
			{Name: "email", Type: "varchar", Length: 255, Nullable: boolPtr(true)},
		}},
	}}
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeFieldModified, TableName: "users", FieldName: "email"},
		},
	}

	src, err := gen.GenerateMigration("0005_alter", nil, diff, newSchema, oldSchema, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `alter_field(`)
	assertContains(t, src, `old_field = field("email", "varchar"`)
	assertContains(t, src, `new_field = field("email", "varchar"`)
}

func TestStarlarkGenerator_RenameField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeFieldRenamed, TableName: "users", FieldName: "fname", NewValue: "first_name"},
		},
	}

	src, err := gen.GenerateMigration("0006_rename", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `rename_field(table="users", old_name="fname", new_name="first_name")`)
}

func TestStarlarkGenerator_RenameTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRenamed, TableName: "old_users", NewValue: "users"},
		},
	}

	src, err := gen.GenerateMigration("0007_rename_tbl", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `rename_table(old_name="old_users", new_name="users")`)
}

func TestStarlarkGenerator_AddIndex(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeIndexAdded,
				TableName: "users",
				NewValue:  yaml.Index{Name: "idx_email", Fields: []string{"email"}, Unique: true},
			},
		},
	}

	src, err := gen.GenerateMigration("0008_add_idx", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `add_index("users"`)
	assertContains(t, src, `index("idx_email"`)
	assertContains(t, src, `unique=True`)
}

func TestStarlarkGenerator_DropIndex(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeIndexRemoved, TableName: "users", FieldName: "idx_email"},
		},
	}

	src, err := gen.GenerateMigration("0009_drop_idx", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `drop_index(table="users", index_name="idx_email")`)
}

func TestStarlarkGenerator_SetDefaults(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeDefaultsModified, NewValue: map[string]string{
				"new_uuid": "gen_random_uuid()",
				"now":      "now()",
			}},
		},
	}

	src, err := gen.GenerateMigration("0010_defaults", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `set_defaults(`)
	assertContains(t, src, `"new_uuid": "gen_random_uuid()"`)
	assertContains(t, src, `"now": "now()"`)
}

func TestStarlarkGenerator_SetTypeMappings(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTypeMappingsModified, NewValue: map[string]string{
				"text": "longtext",
			}},
		},
	}

	src, err := gen.GenerateMigration("0011_tmappings", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `set_type_mappings(`)
	assertContains(t, src, `"text": "longtext"`)
}

func TestStarlarkGenerator_AddForeignKey(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:      yaml.ChangeTypeForeignKeyAdded,
				TableName: "orders",
				FieldName: "user_id",
				NewValue: yaml.Field{
					Name: "user_id",
					Type: "foreign_key",
					ForeignKey: &yaml.ForeignKey{
						Table:    "users",
						OnDelete: "CASCADE",
					},
				},
			},
		},
	}

	src, err := gen.GenerateMigration("0012_fk", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `add_foreign_key(`)
	assertContains(t, src, `table = "orders"`)
	assertContains(t, src, `field_name = "user_id"`)
	assertContains(t, src, `referenced_table = "users"`)
	assertContains(t, src, `on_delete = "CASCADE"`)
}

func TestStarlarkGenerator_DropForeignKey(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeForeignKeyRemoved, TableName: "orders", FieldName: "user_id"},
		},
	}

	src, err := gen.GenerateMigration("0013_drop_fk", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `drop_foreign_key(table="orders"`)
}

func TestStarlarkGenerator_ReviewComment(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRemoved, TableName: "risky_table"},
		},
	}
	decisions := map[int]yaml.PromptResponse{0: yaml.PromptReview}

	src, err := gen.GenerateMigration("0014_review", nil, diff, nil, nil, decisions)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `# REVIEW`)
}

func TestStarlarkGenerator_GenerateBlank(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	src, err := gen.GenerateBlank("0015_custom", []string{"0014_review"})
	if err != nil {
		t.Fatalf("GenerateBlank: %v", err)
	}
	assertContains(t, src, `migration(`)
	assertContains(t, src, `name = "0015_custom"`)
	assertContains(t, src, `"0014_review"`)
	assertContains(t, src, `# TODO`)
}

func TestStarlarkGenerator_FileExtension(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	if ext := gen.FileExtension(); ext != ".star" {
		t.Errorf("expected .star, got %q", ext)
	}
}

func TestStarlarkGenerator_NilDiff(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	_, err := gen.GenerateMigration("test", nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil diff")
	}
}

func TestStarlarkGenerator_MultipleDeps(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRemoved, TableName: "t"},
		},
	}

	src, err := gen.GenerateMigration("0016_multi", []string{"0001_a", "0002_b"}, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `"0001_a"`)
	assertContains(t, src, `"0002_b"`)
}

func TestStarlarkGenerator_IgnoreErrors(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{Type: yaml.ChangeTypeTableRemoved, TableName: "maybe_gone"},
		},
	}
	decisions := map[int]yaml.PromptResponse{0: yaml.PromptIgnoreErrors}

	src, err := gen.GenerateMigration("0017_ignore", nil, diff, nil, nil, decisions)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `ignore_errors=True`)
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q\nGot:\n%s", substr, s)
	}
}
