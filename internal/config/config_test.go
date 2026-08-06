package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Database.Type != "postgresql" {
		t.Errorf("expected database type 'postgresql', got %q", cfg.Database.Type)
	}
	if cfg.Database.DefaultURL != "" {
		t.Errorf("expected default_url to be empty by default, got %q", cfg.Database.DefaultURL)
	}
	if cfg.Migration.Directory != "migrations" {
		t.Errorf("expected migration directory 'migrations', got %q", cfg.Migration.Directory)
	}
	if cfg.Output.Verbose {
		t.Error("expected verbose to be false by default")
	}
	if !cfg.Output.ColorEnabled {
		t.Error("expected color_enabled to be true by default")
	}
}

func TestGetConfigPath(t *testing.T) {
	expected := filepath.Join("migrations", "morphic.config.yaml")
	got := GetConfigPath()
	if got != expected {
		t.Errorf("expected config path %q, got %q", expected, got)
	}
}

func TestLoadValidConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: mysql
  default_url: "host=localhost port=3306 dbname=testdb user=root"
migration:
  directory: db/migrations
output:
  verbose: true
  color_enabled: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.Type != "mysql" {
		t.Errorf("expected database type 'mysql', got %q", cfg.Database.Type)
	}
	if cfg.Database.DefaultURL != "host=localhost port=3306 dbname=testdb user=root" {
		t.Errorf("expected default_url 'host=localhost port=3306 dbname=testdb user=root', got %q", cfg.Database.DefaultURL)
	}
	if cfg.Migration.Directory != "db/migrations" {
		t.Errorf("expected migration directory 'db/migrations', got %q", cfg.Migration.Directory)
	}
	if !cfg.Output.Verbose {
		t.Error("expected verbose to be true")
	}
	if cfg.Output.ColorEnabled {
		t.Error("expected color_enabled to be false")
	}
}

func TestLoadPartialConfigUsesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Only set database type; other fields should get defaults
	content := `database:
  type: sqlite
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.Type != "sqlite" {
		t.Errorf("expected database type 'sqlite', got %q", cfg.Database.Type)
	}
	// These should still be defaults
	if cfg.Migration.Directory != "migrations" {
		t.Errorf("expected default migration directory 'migrations', got %q", cfg.Migration.Directory)
	}
	if !cfg.Output.ColorEnabled {
		t.Error("expected default color_enabled to be true")
	}
}

func TestLoadDefaultsWhenFileDoesNotExist(t *testing.T) {
	// Pass an empty config path so viper searches for files that don't exist
	// in the current temp directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Should get all defaults
	defaults := DefaultConfig()
	if cfg.Database.Type != defaults.Database.Type {
		t.Errorf("expected default database type %q, got %q", defaults.Database.Type, cfg.Database.Type)
	}
	if cfg.Migration.Directory != defaults.Migration.Directory {
		t.Errorf("expected default migration directory %q, got %q", defaults.Migration.Directory, cfg.Migration.Directory)
	}
	if cfg.Output.Verbose != defaults.Output.Verbose {
		t.Errorf("expected default verbose %v, got %v", defaults.Output.Verbose, cfg.Output.Verbose)
	}
	if cfg.Output.ColorEnabled != defaults.Output.ColorEnabled {
		t.Errorf("expected default color_enabled %v, got %v", defaults.Output.ColorEnabled, cfg.Output.ColorEnabled)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: [invalid yaml here
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadOrDefault(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		wantDBType string
	}{
		{
			name: "returns config when file exists",
			setup: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				cfgPath := filepath.Join(tmpDir, "config.yaml")
				content := `database:
  type: sqlserver
`
				if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write test config: %v", err)
				}
				return cfgPath
			},
			wantDBType: "sqlserver",
		},
		{
			name: "returns defaults when file is invalid",
			setup: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				cfgPath := filepath.Join(tmpDir, "config.yaml")
				content := `database: [broken`
				if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write test config: %v", err)
				}
				return cfgPath
			},
			wantDBType: "postgresql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := tt.setup(t)
			cfg := LoadOrDefault(cfgPath)
			if cfg.Database.Type != tt.wantDBType {
				t.Errorf("expected database type %q, got %q", tt.wantDBType, cfg.Database.Type)
			}
		})
	}
}

func TestSaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	original := &Config{
		Database: DatabaseConfig{
			Type:       "mysql",
			DefaultURL: "host=localhost port=3306 dbname=testdb user=root",
		},
		Migration: MigrationConfig{Directory: "my_migrations"},
		Output: OutputConfig{
			Verbose:      true,
			ColorEnabled: false,
		},
	}

	if err := original.Save(cfgPath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("expected config file to be created")
	}

	// Verify the file contains the header comment
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("saved config file is empty")
	}
	if string(data[:len("# Morphic")]) != "# Morphic" {
		t.Error("expected config file to start with header comment")
	}

	// Reload and verify round-trip
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error after Save: %v", err)
	}

	if loaded.Database.Type != original.Database.Type {
		t.Errorf("round-trip database type: expected %q, got %q", original.Database.Type, loaded.Database.Type)
	}
	if loaded.Database.DefaultURL != original.Database.DefaultURL {
		t.Errorf("round-trip default_url: expected %q, got %q", original.Database.DefaultURL, loaded.Database.DefaultURL)
	}
	if loaded.Migration.Directory != original.Migration.Directory {
		t.Errorf("round-trip migration directory: expected %q, got %q", original.Migration.Directory, loaded.Migration.Directory)
	}
	if loaded.Output.Verbose != original.Output.Verbose {
		t.Errorf("round-trip verbose: expected %v, got %v", original.Output.Verbose, loaded.Output.Verbose)
	}
	if loaded.Output.ColorEnabled != original.Output.ColorEnabled {
		t.Errorf("round-trip color_enabled: expected %v, got %v", original.Output.ColorEnabled, loaded.Output.ColorEnabled)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "a", "b", "c", "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Save should create intermediate directories, got error: %v", err)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Fatal("expected config file to be created in nested directory")
	}
}

func TestLoadLegacyConfigFallback(t *testing.T) {
	// When no morphic.config.yaml exists but makemigrations.config.yaml does,
	// the loader should fall back to the legacy name.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	// Create migrations dir with legacy config name only
	migrationsDir := filepath.Join(tmpDir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("failed to create migrations dir: %v", err)
	}
	legacyCfg := filepath.Join(migrationsDir, "makemigrations.config.yaml")
	content := `database:
  type: sqlite
`
	if err := os.WriteFile(legacyCfg, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.Type != "sqlite" {
		t.Errorf("expected legacy fallback to load database type 'sqlite', got %q", cfg.Database.Type)
	}
}

func TestLoadDefaultURLFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: postgresql
  default_url: "host=dbhost port=5432 dbname=myapp user=app sslmode=disable"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	expected := "host=dbhost port=5432 dbname=myapp user=app sslmode=disable"
	if cfg.Database.DefaultURL != expected {
		t.Errorf("expected default_url %q, got %q", expected, cfg.Database.DefaultURL)
	}
}

func TestLoadDefaultURLEmptyByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Config without default_url should leave it empty
	content := `database:
  type: postgresql
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.DefaultURL != "" {
		t.Errorf("expected default_url to be empty when not set, got %q", cfg.Database.DefaultURL)
	}
}

func TestLoadDefaultURLEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: postgresql
  default_url: "host=filehost dbname=filedb"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Env var should override the file value
	t.Setenv("MORPHIC_DATABASE_DEFAULT_URL", "host=envhost dbname=envdb")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.DefaultURL != "host=envhost dbname=envdb" {
		t.Errorf("expected env override for default_url, got %q", cfg.Database.DefaultURL)
	}
}

func TestMigrationFormatFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `migration:
  directory: migrations
  format: starlark
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Migration.Format != "starlark" {
		t.Errorf("expected migration format 'starlark', got %q", cfg.Migration.Format)
	}
}

func TestMigrationFormatDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Migration.Format != "go" {
		t.Errorf("expected default migration format 'go', got %q", cfg.Migration.Format)
	}
}

func TestHasInclude(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HasInclude("example.com/mod", "schema/schema.star") {
		t.Error("expected HasInclude to return false for empty includes")
	}

	cfg.Includes = []IncludeEntry{
		{Module: "example.com/mod", Path: "schema/schema.star"},
	}

	if !cfg.HasInclude("example.com/mod", "schema/schema.star") {
		t.Error("expected HasInclude to return true for existing include")
	}

	if cfg.HasInclude("example.com/other", "schema/schema.star") {
		t.Error("expected HasInclude to return false for different module")
	}

	if cfg.HasInclude("example.com/mod", "other/schema.yaml") {
		t.Error("expected HasInclude to return false for different path")
	}
}

func TestAddInclude(t *testing.T) {
	cfg := DefaultConfig()

	added := cfg.AddInclude("example.com/mod", "schema/schema.star")
	if !added {
		t.Error("expected AddInclude to return true for new entry")
	}
	if len(cfg.Includes) != 1 {
		t.Fatalf("expected 1 include, got %d", len(cfg.Includes))
	}

	added = cfg.AddInclude("example.com/mod", "schema/schema.star")
	if added {
		t.Error("expected AddInclude to return false for duplicate entry")
	}
	if len(cfg.Includes) != 1 {
		t.Fatalf("expected still 1 include after duplicate, got %d", len(cfg.Includes))
	}

	added = cfg.AddInclude("example.com/other", "db/schema.yaml")
	if !added {
		t.Error("expected AddInclude to return true for different entry")
	}
	if len(cfg.Includes) != 2 {
		t.Fatalf("expected 2 includes, got %d", len(cfg.Includes))
	}
}

func TestIncludesSaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "morphic.config.yaml")

	original := DefaultConfig()
	original.AddInclude("example.com/auth", "schema/schema.star")
	original.AddInclude("example.com/billing", "db/schema.yaml")

	if err := original.Save(cfgPath); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(loaded.Includes) != 2 {
		t.Fatalf("expected 2 includes after reload, got %d", len(loaded.Includes))
	}

	if !loaded.HasInclude("example.com/auth", "schema/schema.star") {
		t.Error("expected auth include to survive round-trip")
	}
	if !loaded.HasInclude("example.com/billing", "db/schema.yaml") {
		t.Error("expected billing include to survive round-trip")
	}
}

func TestLoadConfigWithIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: postgresql
includes:
  - module: example.com/users
    path: schema/schema.star
  - module: example.com/orders
    path: db/schema.yaml
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if len(cfg.Includes) != 2 {
		t.Fatalf("expected 2 includes, got %d", len(cfg.Includes))
	}

	if cfg.Includes[0].Module != "example.com/users" || cfg.Includes[0].Path != "schema/schema.star" {
		t.Errorf("unexpected first include: %+v", cfg.Includes[0])
	}
	if cfg.Includes[1].Module != "example.com/orders" || cfg.Includes[1].Path != "db/schema.yaml" {
		t.Errorf("unexpected second include: %+v", cfg.Includes[1])
	}
}

func TestLoadWithEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	content := `database:
  type: postgresql
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Set env var to override
	t.Setenv("MORPHIC_DATABASE_TYPE", "mysql")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Database.Type != "mysql" {
		t.Errorf("expected env override to set database type 'mysql', got %q", cfg.Database.Type)
	}
}
