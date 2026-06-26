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
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ocomsoft/morphic/internal/ui"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
	"github.com/ocomsoft/morphic/migrate"
)

// TestTypeMappingsSurvivedMergeAndDiffDetection is a regression test for the
// bug where TypeMappings in a schema.yaml were silently dropped by MergeSchemas,
// causing morphic generate to report "No changes detected" even when type_mappings
// were present (e.g. float → DOUBLE PRECISION in air_radiators).
//
// The test verifies the full pipeline:
//  1. MergeSchemas preserves TypeMappings from component schemas.
//  2. getTypeMappingsForDB returns the correct values from the merged schema.
//  3. schemaStateToYAMLSchema of a prior DAG state (no SetTypeMappings op) yields empty TypeMappings.
//  4. The diff between prev and current is non-empty, i.e. a SetTypeMappings migration would be generated.
func TestTypeMappingsSurvivedMergeAndDiffDetection(t *testing.T) {
	// schema1 has TypeMappings — mirrors the corecalc/schema/schema.yaml case
	schema1 := &yamlpkg.Schema{
		Database: yamlpkg.Database{Name: "corecalc", Version: "1.0.0"},
		TypeMappings: yamlpkg.TypeMappings{
			yamlpkg.DatabasePostgreSQL: {"float": "DOUBLE PRECISION"},
		},
		Tables: []yamlpkg.Table{
			{Name: "core_data_file", Fields: []yamlpkg.Field{{Name: "id", Type: "uuid", PrimaryKey: true}}},
		},
	}

	// schema2 has no TypeMappings — mirrors a sub-schema (data/ or eng/)
	schema2 := &yamlpkg.Schema{
		Database: yamlpkg.Database{Name: "corecalc", Version: "1.0.0"},
		Tables: []yamlpkg.Table{
			{Name: "unit_type", Fields: []yamlpkg.Field{{Name: "id", Type: "uuid", PrimaryKey: true}}},
		},
	}

	// Step 1: Merge — TypeMappings must survive
	merger := yamlpkg.NewMerger(false)
	merged, err := merger.MergeSchemas([]*yamlpkg.Schema{schema1, schema2})
	if err != nil {
		t.Fatalf("MergeSchemas: %v", err)
	}

	// Step 2: Extract current TypeMappings for PostgreSQL
	currMappings := getTypeMappingsForDB(merged, "postgresql")
	if len(currMappings) == 0 {
		t.Fatal("TypeMappings lost during MergeSchemas — regression: getTypeMappingsForDB returned empty after merge")
	}
	if currMappings["float"] != "DOUBLE PRECISION" {
		t.Errorf("expected float→DOUBLE PRECISION, got %q", currMappings["float"])
	}

	// Step 3: Simulate prior DAG state with no SetTypeMappings op (prevSchema has empty TypeMappings)
	prevState := migrate.NewSchemaState() // no SetTypeMappings applied
	prevSchema := schemaStateToYAMLSchema(prevState, "postgresql")
	prevMappings := getTypeMappingsForDB(prevSchema, "postgresql")

	// Step 4: Diff detection — should see a change
	if mapsEqual(prevMappings, currMappings) {
		t.Error("expected diff to detect TypeMappings change, but mapsEqual returned true")
	}
	if len(currMappings) == 0 {
		t.Error("current TypeMappings must be non-empty to trigger SetTypeMappings generation")
	}
}

// TestBuildMigrationName_WithCustomName verifies that a custom name is
// normalized (lowered, spaces to underscores) and prepended with the
// correct zero-padded sequence number.
func TestBuildMigrationName_WithCustomName(t *testing.T) {
	name := BuildMigrationName(0, "add user phone", "")
	if !strings.HasPrefix(name, "0001_") {
		t.Fatalf("expected prefix '0001_', got %q", name)
	}
	if !strings.Contains(name, "add_user_phone") {
		t.Fatalf("expected 'add_user_phone' in name, got %q", name)
	}
}

// TestBuildMigrationName_WithAutoName verifies that the auto-generated name
// from the diff engine is used when no custom name is provided.
func TestBuildMigrationName_WithAutoName(t *testing.T) {
	name := BuildMigrationName(2, "", "add_email")
	if !strings.HasPrefix(name, "0003_") {
		t.Fatalf("expected prefix '0003_', got %q", name)
	}
	if !strings.HasSuffix(name, "add_email") {
		t.Fatalf("expected suffix 'add_email', got %q", name)
	}
}

// TestBuildMigrationName_Timestamp verifies that when neither custom nor auto
// name is provided, a timestamp suffix is appended.
func TestBuildMigrationName_Timestamp(t *testing.T) {
	name := BuildMigrationName(0, "", "")
	if !strings.HasPrefix(name, "0001_") {
		t.Fatalf("expected prefix '0001_', got %q", name)
	}
	// Should be "0001_" + 14-digit timestamp = 19 chars total
	if len(name) != 19 {
		t.Fatalf("expected 19 chars (0001_ + 14 digit timestamp), got %d: %q", len(name), name)
	}
}

// TestBuildMigrationName_CustomNamePriority verifies that custom name takes
// priority over auto name when both are provided.
func TestBuildMigrationName_CustomNamePriority(t *testing.T) {
	name := BuildMigrationName(5, "my_custom", "auto_name")
	if !strings.HasPrefix(name, "0006_") {
		t.Fatalf("expected prefix '0006_', got %q", name)
	}
	if !strings.Contains(name, "my_custom") {
		t.Fatalf("expected 'my_custom' in name, got %q", name)
	}
	if strings.Contains(name, "auto_name") {
		t.Fatalf("expected auto_name to be ignored, got %q", name)
	}
}

// TestBuildMigrationName_SequenceNumbers verifies that the sequence number is
// correctly derived from the current migration count.
func TestBuildMigrationName_SequenceNumbers(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "0001_"},
		{1, "0002_"},
		{9, "0010_"},
		{99, "0100_"},
		{999, "1000_"},
	}
	for _, tt := range tests {
		name := BuildMigrationName(tt.count, "test", "")
		if !strings.HasPrefix(name, tt.expected) {
			t.Errorf("count=%d: expected prefix %q, got %q", tt.count, tt.expected, name)
		}
	}
}

// TestSchemaStateToYAMLSchema_Nil verifies that a nil SchemaState produces a
// nil yaml.Schema.
func TestSchemaStateToYAMLSchema_Nil(t *testing.T) {
	result := schemaStateToYAMLSchema(nil, "postgresql")
	if result != nil {
		t.Fatal("expected nil result for nil state")
	}
}

// TestSchemaStateToYAMLSchema_EmptyState verifies that an empty SchemaState
// produces a Schema with no tables.
func TestSchemaStateToYAMLSchema_EmptyState(t *testing.T) {
	state := migrate.NewSchemaState()
	result := schemaStateToYAMLSchema(state, "postgresql")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Tables) != 0 {
		t.Fatalf("expected 0 tables, got %d", len(result.Tables))
	}
}

// TestSchemaStateToYAMLSchema_WithTables verifies that tables, fields, and
// indexes are correctly converted from SchemaState to yaml.Schema.
// FK annotation is only present in the output when the constraint exists in
// state — this enables the diff engine to detect missing FK constraints.
func TestSchemaStateToYAMLSchema_WithTables(t *testing.T) {
	state := migrate.NewSchemaState()
	err := state.AddTable("users", []migrate.Field{
		{Name: "id", Type: "uuid", PrimaryKey: true},
		{Name: "name", Type: "varchar", Length: 100, Nullable: true},
		{Name: "org_id", Type: "foreign_key", ForeignKey: &migrate.ForeignKey{
			Table: "orgs", OnDelete: "CASCADE",
		}},
	}, []migrate.Index{
		{Name: "idx_users_name", Fields: []string{"name"}, Unique: false},
	})
	if err != nil {
		t.Fatalf("AddTable: %v", err)
	}

	err = state.AddTable("orgs", []migrate.Field{
		{Name: "id", Type: "uuid", PrimaryKey: true},
	}, nil)
	if err != nil {
		t.Fatalf("AddTable: %v", err)
	}

	// Without an FK constraint in state, the FK annotation must be nil so the
	// diff engine can detect the missing constraint.
	resultNoConstraint := schemaStateToYAMLSchema(state, "postgresql")
	usersTableNoConstraint := resultNoConstraint.Tables[1] // sorted: orgs, users
	orgFieldNoConstraint := usersTableNoConstraint.Fields[2]
	if orgFieldNoConstraint.ForeignKey != nil {
		t.Error("expected ForeignKey to be nil when constraint is absent from state")
	}

	// Add the FK constraint to state — the annotation should now be present.
	if err := state.AddForeignKey("users", migrate.ForeignKeyConstraint{
		Name:            "fk_users_org_id",
		FieldName:       "org_id",
		ReferencedTable: "orgs",
		OnDelete:        "CASCADE",
	}); err != nil {
		t.Fatalf("AddForeignKey: %v", err)
	}

	result := schemaStateToYAMLSchema(state, "postgresql")
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Tables should be sorted alphabetically
	if len(result.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(result.Tables))
	}
	if result.Tables[0].Name != "orgs" {
		t.Fatalf("expected first table to be 'orgs', got %q", result.Tables[0].Name)
	}
	if result.Tables[1].Name != "users" {
		t.Fatalf("expected second table to be 'users', got %q", result.Tables[1].Name)
	}

	// Verify users table fields
	usersTable := result.Tables[1]
	if len(usersTable.Fields) != 3 {
		t.Fatalf("expected 3 fields in users, got %d", len(usersTable.Fields))
	}

	// Verify primary key field
	idField := usersTable.Fields[0]
	if idField.Name != "id" || idField.Type != "uuid" || !idField.PrimaryKey {
		t.Errorf("unexpected id field: %+v", idField)
	}

	// Verify nullable field
	nameField := usersTable.Fields[1]
	if nameField.Nullable == nil || !*nameField.Nullable {
		t.Error("expected name field to be nullable")
	}
	if nameField.Length != 100 {
		t.Errorf("expected name length 100, got %d", nameField.Length)
	}

	// Verify foreign key field — annotation present because constraint exists in state
	orgField := usersTable.Fields[2]
	if orgField.ForeignKey == nil {
		t.Fatal("expected foreign key on org_id field when constraint is in state")
	}
	if orgField.ForeignKey.Table != "orgs" {
		t.Errorf("expected FK table 'orgs', got %q", orgField.ForeignKey.Table)
	}
	if orgField.ForeignKey.OnDelete != "CASCADE" {
		t.Errorf("expected FK on_delete 'CASCADE', got %q", orgField.ForeignKey.OnDelete)
	}

	// Verify indexes
	if len(usersTable.Indexes) != 1 {
		t.Fatalf("expected 1 index on users, got %d", len(usersTable.Indexes))
	}
	idx := usersTable.Indexes[0]
	if idx.Name != "idx_users_name" {
		t.Errorf("expected index name 'idx_users_name', got %q", idx.Name)
	}
	if len(idx.Fields) != 1 || idx.Fields[0] != "name" {
		t.Errorf("unexpected index fields: %v", idx.Fields)
	}
}

// TestSchemaStateToYAMLSchema_FieldAttributes verifies that all field
// attributes (precision, scale, auto_create, auto_update, default) are
// correctly carried through the conversion.
func TestSchemaStateToYAMLSchema_FieldAttributes(t *testing.T) {
	state := migrate.NewSchemaState()
	err := state.AddTable("products", []migrate.Field{
		{
			Name:      "price",
			Type:      "decimal",
			Precision: 10,
			Scale:     2,
			Default:   "0.00",
		},
		{
			Name:       "created_at",
			Type:       "timestamp",
			AutoCreate: true,
		},
		{
			Name:       "updated_at",
			Type:       "timestamp",
			AutoUpdate: true,
		},
	}, nil)
	if err != nil {
		t.Fatalf("AddTable: %v", err)
	}

	result := schemaStateToYAMLSchema(state, "postgresql")
	if result == nil || len(result.Tables) != 1 {
		t.Fatal("expected 1 table")
	}

	fields := result.Tables[0].Fields
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// Verify price field
	price := fields[0]
	if price.Precision != 10 || price.Scale != 2 {
		t.Errorf("price: expected precision=10 scale=2, got precision=%d scale=%d",
			price.Precision, price.Scale)
	}
	if price.Default != "0.00" {
		t.Errorf("price: expected default '0.00', got %q", price.Default)
	}

	// Verify created_at
	createdAt := fields[1]
	if !createdAt.AutoCreate {
		t.Error("created_at: expected auto_create=true")
	}

	// Verify updated_at
	updatedAt := fields[2]
	if !updatedAt.AutoUpdate {
		t.Error("updated_at: expected auto_update=true")
	}
}

// TestGoGenerateMerge_WritesMergeFile verifies that goGenerateMerge produces
// a merge migration .go file with the correct name and dependencies.
func TestGoGenerateMerge_WritesMergeFile(t *testing.T) {
	tmpDir := t.TempDir()
	dagOut := &migrate.DAGOutput{
		Leaves:      []string{"0001_initial", "0002_feature"},
		HasBranches: true,
	}
	err := goGenerateMerge(tmpDir, dagOut, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify a .star file was written
	files, _ := filepath.Glob(filepath.Join(tmpDir, "*.star"))
	if len(files) != 1 {
		t.Fatalf("expected 1 .star file, got %d", len(files))
	}

	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("reading merge file: %v", err)
	}

	// Check that it contains the expected dependencies
	src := string(content)
	if !strings.Contains(src, `"0001_initial"`) {
		t.Error("expected merge file to contain dependency '0001_initial'")
	}
	if !strings.Contains(src, `"0002_feature"`) {
		t.Error("expected merge file to contain dependency '0002_feature'")
	}
	if !strings.Contains(src, "merge") {
		t.Error("expected merge file name to contain 'merge'")
	}
}

// TestGoGenerateMerge_DryRun verifies that goGenerateMerge prints to stdout
// and does not write any file when dryRun is true.
func TestGoGenerateMerge_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	dagOut := &migrate.DAGOutput{
		Leaves:      []string{"0001_a", "0001_b"},
		HasBranches: true,
	}
	err := goGenerateMerge(tmpDir, dagOut, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No file should be written in dry-run mode
	files, _ := filepath.Glob(filepath.Join(tmpDir, "*.star"))
	if len(files) != 0 {
		t.Fatalf("expected 0 .star files in dry-run, got %d", len(files))
	}
}

// TestGoGenerateMerge_LongNameTruncation verifies that merge migration names
// are truncated when they exceed 80 characters.
func TestGoGenerateMerge_LongNameTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	dagOut := &migrate.DAGOutput{
		Leaves: []string{
			"0001_very_long_migration_name_that_goes_on",
			"0002_another_very_long_name_that_also_goes",
		},
		HasBranches: true,
	}
	err := goGenerateMerge(tmpDir, dagOut, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir, "*.star"))
	if len(files) != 1 {
		t.Fatalf("expected 1 .star file, got %d", len(files))
	}
	baseName := filepath.Base(files[0])
	if !strings.Contains(baseName, "merge") {
		t.Errorf("expected filename to contain 'merge', got %q", baseName)
	}
}

// TestSchemaStateToYAMLSchema_TypeMappings verifies that TypeMappings are repopulated
// from state into the correct provider field of the returned schema.
func TestSchemaStateToYAMLSchema_TypeMappings(t *testing.T) {
	state := migrate.NewSchemaState()
	state.SetTypeMappings(map[string]string{"float": "DOUBLE PRECISION"})

	result := schemaStateToYAMLSchema(state, "postgresql")
	if result == nil {
		t.Fatal("expected non-nil schema")
	}
	if result.TypeMappings.ForProvider(yamlpkg.DatabasePostgreSQL)["float"] != "DOUBLE PRECISION" {
		t.Errorf("expected DOUBLE PRECISION in PostgreSQL TypeMappings, got %q",
			result.TypeMappings.ForProvider(yamlpkg.DatabasePostgreSQL)["float"])
	}
	if len(result.TypeMappings.ForProvider(yamlpkg.DatabaseMySQL)) > 0 {
		t.Error("MySQL TypeMappings should be empty")
	}
}

func TestSchemaStateToYAMLSchema_TypeMappings_MySQL(t *testing.T) {
	state := migrate.NewSchemaState()
	state.SetTypeMappings(map[string]string{"text": "LONGTEXT"})

	result := schemaStateToYAMLSchema(state, "mysql")
	if result.TypeMappings.ForProvider(yamlpkg.DatabaseMySQL)["text"] != "LONGTEXT" {
		t.Errorf("expected LONGTEXT in MySQL TypeMappings, got %q", result.TypeMappings.ForProvider(yamlpkg.DatabaseMySQL)["text"])
	}
}

// TestGetTypeMappingsForDB verifies the helper returns the correct provider map.
func TestGetTypeMappingsForDB(t *testing.T) {
	schema := &yamlpkg.Schema{
		TypeMappings: yamlpkg.TypeMappings{
			yamlpkg.DatabasePostgreSQL: {"float": "DOUBLE PRECISION"},
			yamlpkg.DatabaseMySQL:      {"text": "LONGTEXT"},
		},
	}

	pg := getTypeMappingsForDB(schema, "postgresql")
	if pg["float"] != "DOUBLE PRECISION" {
		t.Errorf("pg: expected DOUBLE PRECISION, got %q", pg["float"])
	}
	my := getTypeMappingsForDB(schema, "mysql")
	if my["text"] != "LONGTEXT" {
		t.Errorf("mysql: expected LONGTEXT, got %q", my["text"])
	}
	if got := getTypeMappingsForDB(nil, "postgresql"); got != nil {
		t.Errorf("nil schema should return nil, got %v", got)
	}
}

// TestSchemaStateToYAMLSchema_WithDefaults verifies that state Defaults are
// populated into the correct DB-type slot of the returned yaml.Schema.
func TestSchemaStateToYAMLSchema_WithDefaults(t *testing.T) {
	state := migrate.NewSchemaState()
	state.SetDefaults(map[string]string{
		"uuid": "uuid_generate_v4()",
		"now":  "CURRENT_TIMESTAMP",
	})

	result := schemaStateToYAMLSchema(state, "postgresql")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Defaults.ForProvider(yamlpkg.DatabasePostgreSQL)["uuid"] != "uuid_generate_v4()" {
		t.Errorf("expected uuid default in PostgreSQL slot, got %q", result.Defaults.ForProvider(yamlpkg.DatabasePostgreSQL)["uuid"])
	}
	if result.Defaults.ForProvider(yamlpkg.DatabasePostgreSQL)["now"] != "CURRENT_TIMESTAMP" {
		t.Errorf("expected now default in PostgreSQL slot, got %q", result.Defaults.ForProvider(yamlpkg.DatabasePostgreSQL)["now"])
	}
	// Other DB types should be empty
	if len(result.Defaults.ForProvider(yamlpkg.DatabaseMySQL)) != 0 {
		t.Errorf("expected empty MySQL defaults, got %v", result.Defaults.ForProvider(yamlpkg.DatabaseMySQL))
	}
}

func TestParsePromptInput(t *testing.T) {
	tests := []struct {
		input     string
		wantResp  yamlpkg.PromptResponse
		wantScope ui.PromptScope
	}{
		{"1", yamlpkg.PromptGenerate, ui.ScopeOne},
		{"2", yamlpkg.PromptReview, ui.ScopeOne},
		{"3", yamlpkg.PromptOmit, ui.ScopeOne},
		{"4", yamlpkg.PromptExit, ui.ScopeOne},
		{"5", yamlpkg.PromptIgnoreErrors, ui.ScopeOne},
		{"1a", yamlpkg.PromptGenerate, ui.ScopeAll},
		{"1A", yamlpkg.PromptGenerate, ui.ScopeAll},
		{"3a", yamlpkg.PromptOmit, ui.ScopeAll},
		{"5a", yamlpkg.PromptIgnoreErrors, ui.ScopeAll},
		{"1t", yamlpkg.PromptGenerate, ui.ScopeType},
		{"1T", yamlpkg.PromptGenerate, ui.ScopeType},
		{"2t", yamlpkg.PromptReview, ui.ScopeType},
		{"3t", yamlpkg.PromptOmit, ui.ScopeType},
		{"5t", yamlpkg.PromptIgnoreErrors, ui.ScopeType},
		{"4a", yamlpkg.PromptExit, ui.ScopeOne},   // exit ignores scope
		{"", yamlpkg.PromptGenerate, ui.ScopeOne}, // empty defaults to generate
		{"x", yamlpkg.PromptGenerate, ui.ScopeOne},
	}
	for _, tt := range tests {
		resp, scope := ui.ParsePromptInput(tt.input)
		if resp != tt.wantResp || scope != tt.wantScope {
			t.Errorf("ParsePromptInput(%q) = (%d, %d), want (%d, %d)",
				tt.input, resp, scope, tt.wantResp, tt.wantScope)
		}
	}
}
