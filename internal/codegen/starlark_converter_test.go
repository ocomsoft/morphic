package codegen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/migrate"
)

// TestConvertMigrationToStarlark_RoundTrip converts a Go migration to Starlark
// then loads it back and verifies the operations match.
func TestConvertMigrationToStarlark_RoundTrip(t *testing.T) {
	m := &migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.SetDefaults{
				Defaults: map[string]string{
					"new_uuid": "gen_random_uuid()",
					"now":      "now()",
				},
			},
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true, Default: "new_uuid"},
					{Name: "email", Type: "varchar", Length: 255, Nullable: true},
					{Name: "created_date", Type: "timestamp", Default: "now", AutoCreate: true},
				},
				Indexes: []migrate.Index{
					{Name: "users_email_idx", Fields: []string{"email"}, Unique: true},
				},
			},
			&migrate.RunSQL{
				ForwardSQL:  "CREATE EXTENSION IF NOT EXISTS pgcrypto",
				BackwardSQL: "DROP EXTENSION IF EXISTS pgcrypto",
			},
		},
	}

	src, err := codegen.ConvertMigrationToStarlark(m)
	if err != nil {
		t.Fatalf("ConvertMigrationToStarlark: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v\n\nSource:\n%s", err, src)
	}

	loaded := reg.All()
	if len(loaded) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(loaded))
	}

	lm := loaded[0]
	if lm.Name != "0001_initial" {
		t.Errorf("expected name 0001_initial, got %q", lm.Name)
	}
	if len(lm.Operations) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(lm.Operations))
	}

	sd, ok := lm.Operations[0].(*migrate.SetDefaults)
	if !ok {
		t.Fatalf("op[0] expected SetDefaults, got %T", lm.Operations[0])
	}
	if sd.Defaults["new_uuid"] != "gen_random_uuid()" {
		t.Errorf("expected new_uuid default")
	}

	ct, ok := lm.Operations[1].(*migrate.CreateTable)
	if !ok {
		t.Fatalf("op[1] expected CreateTable, got %T", lm.Operations[1])
	}
	if ct.Name != "users" {
		t.Errorf("expected table users, got %q", ct.Name)
	}
	if len(ct.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(ct.Fields))
	}
	if len(ct.Indexes) != 1 {
		t.Errorf("expected 1 index, got %d", len(ct.Indexes))
	}

	rs, ok := lm.Operations[2].(*migrate.RunSQL)
	if !ok {
		t.Fatalf("op[2] expected RunSQL, got %T", lm.Operations[2])
	}
	if rs.ForwardSQL != "CREATE EXTENSION IF NOT EXISTS pgcrypto" {
		t.Errorf("forward SQL mismatch: %q", rs.ForwardSQL)
	}
}

// TestConvertMigrationToStarlark_UpsertData tests upsert_data conversion.
func TestConvertMigrationToStarlark_UpsertData(t *testing.T) {
	m := &migrate.Migration{
		Name:         "0002_seed",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.UpsertData{
				Table:        "codes",
				ConflictKeys: []string{"id"},
				Rows: []map[string]any{
					{"id": "abc-123", "name": "Test", "active": true},
					{"id": "def-456", "name": "Other", "active": false},
				},
			},
		},
	}

	src, err := codegen.ConvertMigrationToStarlark(m)
	if err != nil {
		t.Fatalf("ConvertMigrationToStarlark: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0002_seed.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v\n\nSource:\n%s", err, src)
	}

	loaded := reg.All()
	ud, ok := loaded[0].Operations[0].(*migrate.UpsertData)
	if !ok {
		t.Fatalf("expected UpsertData, got %T", loaded[0].Operations[0])
	}
	if ud.Table != "codes" {
		t.Errorf("expected table codes, got %q", ud.Table)
	}
	if len(ud.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(ud.Rows))
	}
}

// TestConvertAirRadiators_AllFiles loads the converted AirRadiators .star files
// if the output directory exists (from a prior convert run).
func TestConvertAirRadiators_AllFiles(t *testing.T) {
	dir := "/workspaces/ocom/go/air_radiators/air_radiators_app/migrations_starlark"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("migrations_starlark directory not found — run `morphic convert` first")
	}

	reg, err := interp.LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 25 {
		t.Errorf("expected 25 migrations, got %d", len(migrations))
	}

	for _, m := range migrations {
		if m.Name == "" {
			t.Error("migration has empty name")
		}
		if len(m.Operations) == 0 {
			t.Errorf("migration %s has no operations", m.Name)
		}
	}
}
