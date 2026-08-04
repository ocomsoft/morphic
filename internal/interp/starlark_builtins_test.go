package interp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/migrate"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func execStar(t *testing.T, src string) *StarlarkBuiltins {
	t.Helper()
	builtins := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", src, builtins.Env())
	if err != nil {
		t.Fatalf("starlark exec failed: %v", err)
	}
	return builtins
}

func TestStarlarkBuiltin_Field_Simple(t *testing.T) {
	builtins := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}

	result, err := starlark.Call(
		thread,
		builtins.Env()["field"],
		starlark.Tuple{starlark.String("id"), starlark.String("uuid")},
		[]starlark.Tuple{
			{starlark.String("primary_key"), starlark.Bool(true)},
			{starlark.String("default"), starlark.String("new_uuid")},
		},
	)
	if err != nil {
		t.Fatalf("field() error: %v", err)
	}

	dict, ok := result.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", result)
	}

	name, _, _ := dict.Get(starlark.String("name"))
	if name != starlark.String("id") {
		t.Errorf("expected name 'id', got %v", name)
	}

	pk, _, _ := dict.Get(starlark.String("primary_key"))
	if pk != starlark.Bool(true) {
		t.Errorf("expected primary_key True, got %v", pk)
	}

	def, _, _ := dict.Get(starlark.String("default"))
	if def != starlark.String("new_uuid") {
		t.Errorf("expected default 'new_uuid', got %v", def)
	}
}

func TestStarlarkBuiltin_Field_WithForeignKey(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        add_field("orders", field("user_id", "foreign_key",
            nullable=True,
            foreign_key=fk("auth_user", on_delete="CASCADE"),
        )),
    ],
)
`)
	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}
	af, ok := m.Operations[0].(*migrate.AddField)
	if !ok {
		t.Fatalf("expected *migrate.AddField, got %T", m.Operations[0])
	}
	if af.Field.ForeignKey == nil {
		t.Fatal("expected foreign key to be set")
	}
	if af.Field.ForeignKey.Table != "auth_user" {
		t.Errorf("expected FK table 'auth_user', got %q", af.Field.ForeignKey.Table)
	}
	if af.Field.ForeignKey.OnDelete != "CASCADE" {
		t.Errorf("expected FK on_delete 'CASCADE', got %q", af.Field.ForeignKey.OnDelete)
	}
}

func TestStarlarkBuiltin_Migration_CreateTable(t *testing.T) {
	b := execStar(t, `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        create_table(
            name = "users",
            fields = [
                field("id", "uuid", primary_key=True, default="new_uuid"),
                field("email", "varchar", length=255),
            ],
            indexes = [
                index("users_email_idx", ["email"], unique=True),
            ],
        ),
    ],
)
`)

	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if m.Name != "0001_initial" {
		t.Errorf("expected name '0001_initial', got %q", m.Name)
	}
	if len(m.Dependencies) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(m.Dependencies))
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}

	ct, ok := m.Operations[0].(*migrate.CreateTable)
	if !ok {
		t.Fatalf("expected *migrate.CreateTable, got %T", m.Operations[0])
	}
	if ct.Name != "users" {
		t.Errorf("expected table name 'users', got %q", ct.Name)
	}
	if len(ct.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(ct.Fields))
	}
	if ct.Fields[0].Name != "id" || !ct.Fields[0].PrimaryKey {
		t.Errorf("expected field 'id' with primary_key, got %+v", ct.Fields[0])
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

func TestStarlarkBuiltin_Migration_UpsertData(t *testing.T) {
	b := execStar(t, `
migration(
    name = "0002_seed",
    dependencies = ["0001_initial"],
    operations = [
        upsert_data(
            table = "countries",
            conflict_keys = ["code"],
            rows = [
                {"code": "AU", "name": "Australia"},
                {"code": "NZ", "name": "New Zealand"},
            ],
        ),
    ],
)
`)

	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if len(m.Dependencies) != 1 || m.Dependencies[0] != "0001_initial" {
		t.Errorf("expected dependencies ['0001_initial'], got %v", m.Dependencies)
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}

	ud, ok := m.Operations[0].(*migrate.UpsertData)
	if !ok {
		t.Fatalf("expected *migrate.UpsertData, got %T", m.Operations[0])
	}
	if ud.Table != "countries" {
		t.Errorf("expected table 'countries', got %q", ud.Table)
	}
	if len(ud.ConflictKeys) != 1 || ud.ConflictKeys[0] != "code" {
		t.Errorf("expected conflict_keys ['code'], got %v", ud.ConflictKeys)
	}
	if len(ud.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(ud.Rows))
	}
	if ud.Rows[0]["code"] != "AU" {
		t.Errorf("expected row[0].code='AU', got %v", ud.Rows[0]["code"])
	}
}

func TestStarlarkBuiltin_SetDefaults(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        set_defaults({"new_uuid": "gen_random_uuid()", "now": "now()"}),
    ],
)
`)
	m := b.Collected()
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 op, got %d", len(m.Operations))
	}
	sd, ok := m.Operations[0].(*migrate.SetDefaults)
	if !ok {
		t.Fatalf("expected *migrate.SetDefaults, got %T", m.Operations[0])
	}
	if sd.Defaults["new_uuid"] != "gen_random_uuid()" {
		t.Errorf("expected 'gen_random_uuid()', got %q", sd.Defaults["new_uuid"])
	}
	if sd.Defaults["now"] != "now()" {
		t.Errorf("expected 'now()', got %q", sd.Defaults["now"])
	}
}

func TestStarlarkBuiltin_SetTypeMappings(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        set_type_mappings({"text": "longtext", "uuid": "char(36)"}),
    ],
)
`)
	m := b.Collected()
	stm, ok := m.Operations[0].(*migrate.SetTypeMappings)
	if !ok {
		t.Fatalf("expected *migrate.SetTypeMappings, got %T", m.Operations[0])
	}
	if stm.TypeMappings["text"] != "longtext" {
		t.Errorf("expected 'longtext', got %q", stm.TypeMappings["text"])
	}
}

func TestStarlarkBuiltin_DropTable(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        drop_table(name="old_table", schema_only=True),
    ],
)
`)
	m := b.Collected()
	dt, ok := m.Operations[0].(*migrate.DropTable)
	if !ok {
		t.Fatalf("expected *migrate.DropTable, got %T", m.Operations[0])
	}
	if dt.Name != "old_table" {
		t.Errorf("expected 'old_table', got %q", dt.Name)
	}
	if !dt.SchemaOnly {
		t.Error("expected schema_only=true")
	}
}

func TestStarlarkBuiltin_DropField(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        drop_field(table="users", field_name="legacy_col"),
    ],
)
`)
	m := b.Collected()
	df, ok := m.Operations[0].(*migrate.DropField)
	if !ok {
		t.Fatalf("expected *migrate.DropField, got %T", m.Operations[0])
	}
	if df.Table != "users" || df.Field != "legacy_col" {
		t.Errorf("got table=%q field=%q", df.Table, df.Field)
	}
}

func TestStarlarkBuiltin_AlterField(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        alter_field(
            table = "users",
            old_field = field("email", "varchar", length=100),
            new_field = field("email", "varchar", length=255, nullable=True),
        ),
    ],
)
`)
	m := b.Collected()
	af, ok := m.Operations[0].(*migrate.AlterField)
	if !ok {
		t.Fatalf("expected *migrate.AlterField, got %T", m.Operations[0])
	}
	if af.OldField.Length != 100 {
		t.Errorf("expected old length 100, got %d", af.OldField.Length)
	}
	if af.NewField.Length != 255 {
		t.Errorf("expected new length 255, got %d", af.NewField.Length)
	}
	if !af.NewField.Nullable {
		t.Error("expected new field to be nullable")
	}
}

func TestStarlarkBuiltin_RenameField(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        rename_field(table="users", old_name="fname", new_name="first_name"),
    ],
)
`)
	m := b.Collected()
	rf, ok := m.Operations[0].(*migrate.RenameField)
	if !ok {
		t.Fatalf("expected *migrate.RenameField, got %T", m.Operations[0])
	}
	if rf.OldName != "fname" || rf.NewName != "first_name" {
		t.Errorf("expected fname→first_name, got %s→%s", rf.OldName, rf.NewName)
	}
}

func TestStarlarkBuiltin_RenameTable(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        rename_table(old_name="old_users", new_name="users"),
    ],
)
`)
	m := b.Collected()
	rt, ok := m.Operations[0].(*migrate.RenameTable)
	if !ok {
		t.Fatalf("expected *migrate.RenameTable, got %T", m.Operations[0])
	}
	if rt.OldName != "old_users" || rt.NewName != "users" {
		t.Errorf("expected old_users→users, got %s→%s", rt.OldName, rt.NewName)
	}
}

func TestStarlarkBuiltin_AddIndex(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        add_index("users", index("idx_email", ["email"], unique=True)),
    ],
)
`)
	m := b.Collected()
	ai, ok := m.Operations[0].(*migrate.AddIndex)
	if !ok {
		t.Fatalf("expected *migrate.AddIndex, got %T", m.Operations[0])
	}
	if ai.Table != "users" {
		t.Errorf("expected table 'users', got %q", ai.Table)
	}
	if ai.Index.Name != "idx_email" || !ai.Index.Unique {
		t.Errorf("expected unique index idx_email, got %+v", ai.Index)
	}
}

func TestStarlarkBuiltin_DropIndex(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        drop_index(table="users", index_name="idx_email"),
    ],
)
`)
	m := b.Collected()
	di, ok := m.Operations[0].(*migrate.DropIndex)
	if !ok {
		t.Fatalf("expected *migrate.DropIndex, got %T", m.Operations[0])
	}
	if di.Table != "users" || di.Index != "idx_email" {
		t.Errorf("got table=%q index=%q", di.Table, di.Index)
	}
}

func TestStarlarkBuiltin_AddForeignKey(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        add_foreign_key(
            table = "orders",
            field_name = "user_id",
            constraint_name = "fk_orders_user",
            referenced_table = "users",
            on_delete = "CASCADE",
        ),
    ],
)
`)
	m := b.Collected()
	afk, ok := m.Operations[0].(*migrate.AddForeignKey)
	if !ok {
		t.Fatalf("expected *migrate.AddForeignKey, got %T", m.Operations[0])
	}
	if afk.Table != "orders" {
		t.Errorf("expected table 'orders', got %q", afk.Table)
	}
	if afk.ConstraintName != "fk_orders_user" {
		t.Errorf("expected constraint 'fk_orders_user', got %q", afk.ConstraintName)
	}
	if afk.OnDelete != "CASCADE" {
		t.Errorf("expected on_delete CASCADE, got %q", afk.OnDelete)
	}
}

func TestStarlarkBuiltin_DropForeignKey(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        drop_foreign_key(table="orders", constraint_name="fk_orders_user"),
    ],
)
`)
	m := b.Collected()
	dfk, ok := m.Operations[0].(*migrate.DropForeignKey)
	if !ok {
		t.Fatalf("expected *migrate.DropForeignKey, got %T", m.Operations[0])
	}
	if dfk.Table != "orders" || dfk.ConstraintName != "fk_orders_user" {
		t.Errorf("got table=%q constraint=%q", dfk.Table, dfk.ConstraintName)
	}
}

func TestStarlarkBuiltin_RunSQL(t *testing.T) {
	b := execStar(t, `
migration(
    name = "test",
    operations = [
        run_sql(forward="CREATE EXTENSION IF NOT EXISTS pgcrypto", backward="DROP EXTENSION pgcrypto"),
    ],
)
`)
	m := b.Collected()
	rs, ok := m.Operations[0].(*migrate.RunSQL)
	if !ok {
		t.Fatalf("expected *migrate.RunSQL, got %T", m.Operations[0])
	}
	if rs.ForwardSQL != "CREATE EXTENSION IF NOT EXISTS pgcrypto" {
		t.Errorf("unexpected forward SQL: %q", rs.ForwardSQL)
	}
	if rs.BackwardSQL != "DROP EXTENSION pgcrypto" {
		t.Errorf("unexpected backward SQL: %q", rs.BackwardSQL)
	}
}

func TestStarlarkBuiltin_MultipleOperations(t *testing.T) {
	b := execStar(t, `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        set_defaults({"new_uuid": "gen_random_uuid()"}),
        create_table(
            name = "users",
            fields = [
                field("id", "uuid", primary_key=True, default="new_uuid"),
                field("email", "varchar", length=255, nullable=True),
            ],
        ),
        add_index("users", index("idx_users_email", ["email"], unique=True)),
        add_foreign_key(
            table = "orders",
            field_name = "user_id",
            constraint_name = "fk_orders_user",
            referenced_table = "users",
            on_delete = "CASCADE",
        ),
    ],
)
`)
	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if len(m.Operations) != 4 {
		t.Fatalf("expected 4 operations, got %d", len(m.Operations))
	}

	if _, ok := m.Operations[0].(*migrate.SetDefaults); !ok {
		t.Errorf("op[0]: expected SetDefaults, got %T", m.Operations[0])
	}
	if _, ok := m.Operations[1].(*migrate.CreateTable); !ok {
		t.Errorf("op[1]: expected CreateTable, got %T", m.Operations[1])
	}
	if _, ok := m.Operations[2].(*migrate.AddIndex); !ok {
		t.Errorf("op[2]: expected AddIndex, got %T", m.Operations[2])
	}
	if _, ok := m.Operations[3].(*migrate.AddForeignKey); !ok {
		t.Errorf("op[3]: expected AddForeignKey, got %T", m.Operations[3])
	}
}

func TestStarlarkBuiltin_NoMigrationCall(t *testing.T) {
	builtins := NewStarlarkBuiltins()
	if builtins.Collected() != nil {
		t.Error("expected Collected() to be nil before any migration() call")
	}
}

func TestStarlarkBuiltin_AutoCreateOnlyOnDatetime(t *testing.T) {
	// auto_create/auto_update should be accepted on datetime types
	b := execStar(t, `
migration(
    name = "test_datetime",
    operations = [
        create_table("events",
            fields = [
                timestamp("created_at", auto_create=True),
                timestamp("updated_at", auto_update=True),
                date("birth_date", auto_create=True),
                time("start_time", auto_update=True),
            ],
        ),
    ],
)
`)
	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	ct := m.Operations[0].(*migrate.CreateTable)
	if !ct.Fields[0].AutoCreate {
		t.Error("expected timestamp auto_create=True")
	}
	if !ct.Fields[1].AutoUpdate {
		t.Error("expected timestamp auto_update=True")
	}
	if !ct.Fields[2].AutoCreate {
		t.Error("expected date auto_create=True")
	}
	if !ct.Fields[3].AutoUpdate {
		t.Error("expected time auto_update=True")
	}
}

func TestStarlarkBuiltin_AutoCreateRejectedOnNonDatetime(t *testing.T) {
	nonDatetimeTypes := []struct {
		name string
		src  string
	}{
		{"uuid", `uuid("id", auto_create=True)`},
		{"varchar", `varchar("name", 100, auto_create=True)`},
		{"text", `text("bio", auto_create=True)`},
		{"integer", `integer("count", auto_update=True)`},
		{"bigint", `bigint("total", auto_create=True)`},
		{"boolean", `boolean("flag", auto_update=True)`},
		{"float", `float("score", auto_create=True)`},
		{"jsonb", `jsonb("data", auto_create=True)`},
		{"bytes", `bytes("blob", auto_update=True)`},
		{"serial", `serial("seq", auto_create=True)`},
		{"decimal", `decimal("price", 10, 2, auto_create=True)`},
		{"foreign_key", `foreign_key("user_id", fk("users"), auto_create=True)`},
	}

	for _, tc := range nonDatetimeTypes {
		t.Run(tc.name, func(t *testing.T) {
			builtins := NewStarlarkBuiltins()
			thread := &starlark.Thread{Name: "test"}
			src := tc.src
			_, err := starlark.ExecFileOptions(&syntax.FileOptions{}, thread, "test.star", src, builtins.Env())
			if err == nil {
				t.Errorf("%s should reject auto_create/auto_update, but accepted it", tc.name)
			}
		})
	}
}

func TestStarlarkBuiltin_UpsertData_RowsFile(t *testing.T) {
	b := execStar(t, `
migration(
    name = "0002_seed",
    dependencies = ["0001_initial"],
    operations = [
        upsert_data(
            table = "countries",
            conflict_keys = ["code"],
            rows_file = "0002_seed_countries.jsonl",
        ),
    ],
)
`)
	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}
	ud, ok := m.Operations[0].(*migrate.UpsertData)
	if !ok {
		t.Fatalf("expected *migrate.UpsertData, got %T", m.Operations[0])
	}
	if ud.RowsFile != "0002_seed_countries.jsonl" {
		t.Errorf("expected RowsFile='0002_seed_countries.jsonl', got %q", ud.RowsFile)
	}
	if len(ud.Rows) != 0 {
		t.Errorf("expected empty Rows when rows_file is set, got %d rows", len(ud.Rows))
	}
}

func TestStarlarkBuiltin_UpsertData_BothRowsAndRowsFileError(t *testing.T) {
	b := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFileOptions(
		&syntax.FileOptions{}, thread, "test.star",
		[]byte(`
migration(
    name = "0002_seed",
    dependencies = [],
    operations = [
        upsert_data(
            table = "countries",
            conflict_keys = ["code"],
            rows = [{"code": "AU"}],
            rows_file = "data.jsonl",
        ),
    ],
)
`), b.Env())
	if err == nil {
		t.Fatal("expected error when both rows and rows_file are set")
	}
}

func TestStarlarkBuiltin_UpsertData_NeitherRowsNorRowsFileError(t *testing.T) {
	b := NewStarlarkBuiltins()
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFileOptions(
		&syntax.FileOptions{}, thread, "test.star",
		[]byte(`
migration(
    name = "0002_seed",
    dependencies = [],
    operations = [
        upsert_data(
            table = "countries",
            conflict_keys = ["code"],
        ),
    ],
)
`), b.Env())
	if err == nil {
		t.Fatal("expected error when neither rows nor rows_file is set")
	}
}

func TestStarlarkBuiltin_Migration_Initial(t *testing.T) {
	dir := t.TempDir()
	src := `migration(
    name = "0001_initial",
    dependencies = [],
    initial = True,
    operations = [
        create_table("users",
            fields = [
                field("id", "uuid", primary_key=True),
                field("email", "varchar", length=255),
            ],
        ),
    ],
)`
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}
	if !migrations[0].Initial {
		t.Error("expected Initial to be true")
	}
}

func TestStarlarkBuiltin_Migration_InitialDefaultsFalse(t *testing.T) {
	dir := t.TempDir()
	src := `migration(
    name = "0002_add_field",
    dependencies = ["0001_initial"],
    operations = [],
)`
	if err := os.WriteFile(filepath.Join(dir, "0002_add_field.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	migrations := reg.All()
	if migrations[0].Initial {
		t.Error("expected Initial to default to false")
	}
}

// TestStarlarkToGoValue_Scalars guards the scalar conversions, in particular None.
// The None case was previously written as *starlark.NoneType; because starlark.None
// is a value (type NoneType byte) and not a pointer, that case never matched and
// None fell through to the default branch, which stringified it to the literal
// "None" instead of converting it to nil (SQL NULL).
func TestStarlarkToGoValue_Scalars(t *testing.T) {
	tests := []struct {
		name string
		in   starlark.Value
		want any
	}{
		{"string", starlark.String("hello"), "hello"},
		{"int", starlark.MakeInt(42), int64(42)},
		{"float", starlark.Float(1.5), 1.5},
		{"bool", starlark.Bool(true), true},
		{"none", starlark.None, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := starlarkToGoValue(tt.in)
			if got != tt.want {
				t.Errorf("starlarkToGoValue(%v) = %#v (%T), want %#v", tt.in, got, got, tt.want)
			}
		})
	}
}

// TestStarlarkBuiltin_UpsertData_NoneIsNil covers the end-to-end path that broke:
// a None in an upsert row must reach the migration as a nil so it is emitted as
// SQL NULL. Written as the literal string "None" it fails against a timestamp
// column and, worse, silently stores "None" in a nullable text column.
func TestStarlarkBuiltin_UpsertData_NoneIsNil(t *testing.T) {
	b := execStar(t, `
migration(
    name = "0002_seed",
    dependencies = ["0001_initial"],
    operations = [
        upsert_data(
            table = "code_core_layout",
            conflict_keys = ["id"],
            rows = [
                row(id = "1", code = "C", active_end_date = None, original_file_name = None),
            ],
        ),
    ],
)
`)

	m := b.Collected()
	if m == nil {
		t.Fatal("no migration collected")
	}
	if len(m.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(m.Operations))
	}

	ud, ok := m.Operations[0].(*migrate.UpsertData)
	if !ok {
		t.Fatalf("expected *migrate.UpsertData, got %T", m.Operations[0])
	}
	if len(ud.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(ud.Rows))
	}

	for _, col := range []string{"active_end_date", "original_file_name"} {
		v, present := ud.Rows[0][col]
		if !present {
			t.Fatalf("column %q missing from row", col)
		}
		if v != nil {
			t.Errorf("row[%q] = %#v (%T), want nil", col, v, v)
		}
	}

	// Non-None values in the same row must be unaffected.
	if ud.Rows[0]["code"] != "C" {
		t.Errorf("row[\"code\"] = %#v, want \"C\"", ud.Rows[0]["code"])
	}
}
