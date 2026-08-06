package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/config"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

func TestExecuteFindIncludes_WritesToConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal schema.star file so the schema path check passes
	schemaDir := filepath.Join(tmpDir, "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatal(err)
	}
	schemaFile := filepath.Join(schemaDir, "schema.star")
	if err := os.WriteFile(schemaFile, []byte("# placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create an initial config
	cfgPath := filepath.Join(tmpDir, "morphic.config.yaml")
	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	// Build a cobra command with the --schema flag
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("schema", "", "")
	if err := cmd.Flags().Set("schema", schemaFile); err != nil {
		t.Fatal(err)
	}

	dummySchema := &yamlpkg.Schema{
		Database: yamlpkg.Database{Name: "testdb"},
	}

	callbacks := &FindIncludesCallbacks{
		FindLocalSchemaFiles: func() ([]LocalSchemaFile, error) {
			return nil, nil
		},
		SelectLocalSchemaFile: func(schemas []LocalSchemaFile) (string, error) {
			return "", nil
		},
		DiscoverSchemas: func() ([]DiscoveredSchema, error) {
			return []DiscoveredSchema{
				{
					ModulePath:   "example.com/auth",
					RelativePath: "schema/schema.star",
					FullPath:     "/tmp/auth/schema/schema.star",
					Schema:       dummySchema,
					TableCount:   3,
					DatabaseName: "auth_db",
					DatabaseType: "starlark",
				},
				{
					ModulePath:   "example.com/billing",
					RelativePath: "db/schema.yaml",
					FullPath:     "/tmp/billing/db/schema.yaml",
					Schema:       dummySchema,
					TableCount:   5,
					DatabaseName: "billing_db",
					DatabaseType: "yaml",
				},
			}, nil
		},
		SelectSchemasInteractively: func(schemas []DiscoveredSchema) ([]DiscoveredSchema, error) {
			return schemas, nil
		},
	}

	err := ExecuteFindIncludes(cmd, cfgPath, schemaFile, false, true, callbacks)
	if err != nil {
		t.Fatalf("ExecuteFindIncludes returned error: %v", err)
	}

	// Verify includes were written to config, not to the schema file
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if len(loadedCfg.Includes) != 2 {
		t.Fatalf("expected 2 includes in config, got %d", len(loadedCfg.Includes))
	}

	if !loadedCfg.HasInclude("example.com/auth", "schema/schema.star") {
		t.Error("expected auth include in config")
	}
	if !loadedCfg.HasInclude("example.com/billing", "db/schema.yaml") {
		t.Error("expected billing include in config")
	}

	// Verify the schema file was NOT modified
	schemaContent, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(schemaContent) != "# placeholder" {
		t.Errorf("schema file should not have been modified, got: %s", string(schemaContent))
	}
}

func TestExecuteFindIncludes_SkipsExistingIncludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a schema file
	schemaFile := filepath.Join(tmpDir, "schema.star")
	if err := os.WriteFile(schemaFile, []byte("# placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a config with an existing include
	cfgPath := filepath.Join(tmpDir, "morphic.config.yaml")
	cfg := config.DefaultConfig()
	cfg.AddInclude("example.com/auth", "schema/schema.star")
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("schema", "", "")
	if err := cmd.Flags().Set("schema", schemaFile); err != nil {
		t.Fatal(err)
	}

	dummySchema := &yamlpkg.Schema{
		Database: yamlpkg.Database{Name: "testdb"},
	}

	callbacks := &FindIncludesCallbacks{
		FindLocalSchemaFiles:  func() ([]LocalSchemaFile, error) { return nil, nil },
		SelectLocalSchemaFile: func(s []LocalSchemaFile) (string, error) { return "", nil },
		DiscoverSchemas: func() ([]DiscoveredSchema, error) {
			return []DiscoveredSchema{
				{
					ModulePath:   "example.com/auth",
					RelativePath: "schema/schema.star",
					Schema:       dummySchema,
				},
				{
					ModulePath:   "example.com/billing",
					RelativePath: "db/schema.yaml",
					Schema:       dummySchema,
				},
			}, nil
		},
		SelectSchemasInteractively: func(s []DiscoveredSchema) ([]DiscoveredSchema, error) { return s, nil },
	}

	err := ExecuteFindIncludes(cmd, cfgPath, schemaFile, false, true, callbacks)
	if err != nil {
		t.Fatalf("ExecuteFindIncludes returned error: %v", err)
	}

	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if len(loadedCfg.Includes) != 2 {
		t.Fatalf("expected 2 includes (1 existing + 1 new), got %d", len(loadedCfg.Includes))
	}

	if !loadedCfg.HasInclude("example.com/billing", "db/schema.yaml") {
		t.Error("expected new billing include to be added")
	}
}
