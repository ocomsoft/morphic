package codegen_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestConvertMigrationToStarlark_UpsertDataWithRowsFile(t *testing.T) {
	m := &migrate.Migration{
		Name:         "0003_seed",
		Dependencies: []string{"0002_initial"},
		Operations: []migrate.Operation{
			&migrate.UpsertData{
				Table:        "countries",
				ConflictKeys: []string{"code"},
				RowsFile:     "0003_seed_countries.jsonl",
			},
		},
	}

	src, err := codegen.ConvertMigrationToStarlark(m)
	if err != nil {
		t.Fatalf("ConvertMigrationToStarlark: %v", err)
	}
	if !strings.Contains(src, `rows_file = "0003_seed_countries.jsonl"`) {
		t.Errorf("expected rows_file in output, got:\n%s", src)
	}
	if strings.Contains(src, "rows = [") {
		t.Errorf("should not contain inline rows when rows_file is set, got:\n%s", src)
	}
}

func TestConvertMigrationToStarlark_InitialTrue(t *testing.T) {
	m := &migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Initial:      true,
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
				},
			},
		},
	}

	src, err := codegen.ConvertMigrationToStarlark(m)
	if err != nil {
		t.Fatalf("ConvertMigrationToStarlark: %v", err)
	}
	if !strings.Contains(src, "initial = True") {
		t.Errorf("expected 'initial = True' in output, got:\n%s", src)
	}
}

func TestConvertMigrationToStarlark_InitialFalse(t *testing.T) {
	m := &migrate.Migration{
		Name:         "0002_add_field",
		Dependencies: []string{"0001_add_base_tables"},
		Initial:      false,
		Operations:   []migrate.Operation{},
	}

	src, err := codegen.ConvertMigrationToStarlark(m)
	if err != nil {
		t.Fatalf("ConvertMigrationToStarlark: %v", err)
	}
	if strings.Contains(src, "initial") {
		t.Errorf("should not contain 'initial' when false, got:\n%s", src)
	}
}

// TestConvertMigrationToStarlark_AlterFieldStrategy verifies that a non-default
// AlterField.Strategy (drop_create) is emitted as a strategy kwarg, while the
// default cast strategy and an unset strategy are omitted from the output.
func TestConvertMigrationToStarlark_AlterFieldStrategy(t *testing.T) {
	tests := []struct {
		name         string
		strategy     migrate.AlterFieldStrategy
		wantContains string
		wantAbsent   string
	}{
		{
			name:         "drop_create strategy is emitted",
			strategy:     migrate.AlterStrategyDropCreate,
			wantContains: `strategy = "drop_create"`,
		},
		{
			name:       "empty strategy is omitted",
			strategy:   "",
			wantAbsent: "strategy =",
		},
		{
			name:       "cast strategy is omitted",
			strategy:   migrate.AlterStrategyCast,
			wantAbsent: "strategy =",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &migrate.Migration{
				Name:         "0007_alter_strategy",
				Dependencies: []string{},
				Operations: []migrate.Operation{
					&migrate.AlterField{
						Table:    "users",
						OldField: migrate.Field{Name: "email", Type: "varchar", Length: 100},
						NewField: migrate.Field{Name: "email", Type: "varchar", Length: 255},
						Strategy: tt.strategy,
					},
				},
			}

			src, err := codegen.ConvertMigrationToStarlark(m)
			if err != nil {
				t.Fatalf("ConvertMigrationToStarlark: %v", err)
			}

			if tt.wantContains != "" && !strings.Contains(src, tt.wantContains) {
				t.Errorf("expected output to contain %q, got:\n%s", tt.wantContains, src)
			}
			if tt.wantAbsent != "" && strings.Contains(src, tt.wantAbsent) {
				t.Errorf("expected output NOT to contain %q, got:\n%s", tt.wantAbsent, src)
			}
		})
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
