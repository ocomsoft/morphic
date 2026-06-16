package interp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ocomsoft/morphic/migrate"
)

func TestLoadStarlarkRegistry_SingleFile(t *testing.T) {
	dir := t.TempDir()
	script := `
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
        ),
    ],
)
`
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry error: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}
	if migrations[0].Name != "0001_initial" {
		t.Errorf("expected name '0001_initial', got %q", migrations[0].Name)
	}
	if len(migrations[0].Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(migrations[0].Operations))
	}
	if _, ok := migrations[0].Operations[0].(*migrate.CreateTable); !ok {
		t.Errorf("expected CreateTable, got %T", migrations[0].Operations[0])
	}
}

func TestLoadStarlarkRegistry_MultipleDeps(t *testing.T) {
	dir := t.TempDir()

	script1 := `
migration(
    name = "0001_initial",
    dependencies = [],
    operations = [
        set_defaults({"new_uuid": "gen_random_uuid()"}),
        create_table(name = "users", fields = [
            field("id", "uuid", primary_key=True, default="new_uuid"),
        ]),
    ],
)
`
	script2 := `
migration(
    name = "0002_add_email",
    dependencies = ["0001_initial"],
    operations = [
        add_field("users", field("email", "varchar", length=255)),
    ],
)
`
	if err := os.WriteFile(filepath.Join(dir, "0001_initial.star"), []byte(script1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0002_add_email.star"), []byte(script2), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry error: %v", err)
	}

	migrations := reg.All()
	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}
}

func TestLoadStarlarkRegistry_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadStarlarkRegistry(dir)
	if err != nil {
		t.Fatalf("LoadStarlarkRegistry error: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(reg.All()))
	}
}

func TestLoadStarlarkRegistry_NoMigrationCall(t *testing.T) {
	dir := t.TempDir()
	script := `x = 1 + 2`
	if err := os.WriteFile(filepath.Join(dir, "bad.star"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadStarlarkRegistry(dir)
	if err == nil {
		t.Fatal("expected error for script without migration() call")
	}
}

func TestLoadRegistry_AutoDetect_Starlark(t *testing.T) {
	dir := t.TempDir()
	script := `
migration(
    name = "0001_test",
    operations = [
        create_table(name = "t", fields = [field("id", "integer", primary_key=True)]),
    ],
)
`
	if err := os.WriteFile(filepath.Join(dir, "0001_test.star"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}
	if len(reg.All()) != 1 {
		t.Errorf("expected 1 migration, got %d", len(reg.All()))
	}
}

func TestLoadRegistry_MixedFormatsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001.go"), []byte("package main\nfunc init() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0002.star"), []byte("migration(name='t', operations=[])"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for mixed formats")
	}
}

func TestLoadRegistry_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(reg.All()))
	}
}
