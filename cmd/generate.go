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

// Package cmd contains all CLI commands for morphic.
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/ui"
	"github.com/ocomsoft/morphic/internal/utils"
	"github.com/ocomsoft/morphic/internal/workflow"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
	"github.com/ocomsoft/morphic/migrate"
)

// Flag variables for the go_migrations command. These are prefixed with goMig
// to avoid conflicts with the root command flag variables.
var (
	goMigDryRun      bool
	goMigCheck       bool
	goMigMerge       bool
	goMigName        string
	goMigVerbose     bool
	goMigAutoApprove bool
	goMigJSON        bool
	goMigFormat      string
)

// DryRunReport is the JSON structure emitted by --dry-run --json.
type DryRunReport struct {
	MigrationName    string         `json:"migration_name"`
	Dependencies     []string       `json:"dependencies"`
	HasDestructive   bool           `json:"has_destructive"`
	DestructiveCount int            `json:"destructive_count"`
	Changes          []DryRunChange `json:"changes"`
	Source           string         `json:"source"`
}

// DryRunChange is a single change entry in the dry-run JSON report.
type DryRunChange struct {
	Type        string `json:"type"`
	Table       string `json:"table"`
	Field       string `json:"field,omitempty"`
	Destructive bool   `json:"destructive"`
	Description string `json:"description"`
}

// goMigrationsCmd is the Cobra command for generating Starlark migration files
// from schema changes. It compares the current schema against the reconstructed
// state from existing migration files and generates a new .star migration file
// for any changes detected.
var goMigrationsCmd = &cobra.Command{
	Use:     "generate",
	GroupID: "schema",
	Short:   "Generate migration files from schema changes",
	Long: `Compares the current schema against the reconstructed state from existing
migration files and generates a new Starlark .star migration file for any
changes detected.

This command implements the Django-style migration workflow:
  1. Loads existing migrations to reconstruct the current schema state
  2. Parses the current schema files
  3. Diffs the two schemas
  4. Generates a new .star migration file with typed operations

Use --merge to generate a merge migration when concurrent branches are detected.
Use --check in CI/CD to fail if unapplied schema changes exist.`,
	RunE: runGoMakeMigrations,
}

func init() {
	rootCmd.AddCommand(goMigrationsCmd)
	goMigrationsCmd.Flags().BoolVar(&goMigDryRun, "dry-run", false,
		"Print generated migration without writing")
	goMigrationsCmd.Flags().BoolVar(&goMigCheck, "check", false,
		"Exit with error if migrations are needed (for CI/CD)")
	goMigrationsCmd.Flags().BoolVar(&goMigMerge, "merge", false,
		"Generate merge migration for detected branches")
	goMigrationsCmd.Flags().StringVar(&goMigName, "name", "",
		"Custom migration name suffix")
	goMigrationsCmd.Flags().BoolVar(&goMigVerbose, "verbose", false,
		"Show detailed output")
	goMigrationsCmd.Flags().BoolVar(&goMigAutoApprove, "auto-approve", false,
		"Automatically approve all destructive operations without prompting (for CI/non-TTY environments)")
	goMigrationsCmd.Flags().BoolVar(&goMigJSON, "json", false,
		"Output dry-run results as JSON (requires --dry-run)")
	goMigrationsCmd.Flags().StringVar(&goMigFormat, "format", "",
		"Output format: go or starlark (overrides config)")
}

// runGoMakeMigrations is the main entry point for migration generation.
// It orchestrates the load-query-diff-generate pipeline:
//  1. Scan for existing migration files in the migrations directory
//  2. If migrations exist, load them and query the DAG for the current schema state
//  3. Parse and merge the current schema files
//  4. Diff the reconstructed state against the current schema
//  5. Generate a new .star migration file (or merge migration if --merge is set)
func runGoMakeMigrations(cmd *cobra.Command, _ []string) error {
	cfg := config.LoadOrDefault(cfgFile)
	if goMigJSON && !goMigDryRun {
		return fmt.Errorf("--json requires --dry-run")
	}
	migrationsDir := cfg.Migration.Directory

	// Override format from --format flag if provided.
	if goMigFormat != "" {
		cfg.Migration.Format = goMigFormat
	}
	gen := newGenerator(cfg)

	// 1. Query existing DAG (if any migration files exist)
	var dagOut *migrate.DAGOutput
	var prevSchema *yamlpkg.Schema

	migFiles, err := countMigrationFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations directory: %w", err)
	}

	if len(migFiles) > 0 {
		dagOut, err = queryDAG(migrationsDir, goMigVerbose)
		if err != nil {
			return fmt.Errorf("querying migration DAG: %w", err)
		}
		prevSchema = schemaStateToYAMLSchema(dagOut.SchemaState, cfg.Database.Type)
	}

	// 2. Parse current schema
	dbType, err := yamlpkg.ParseDatabaseType(cfg.Database.Type)
	if err != nil {
		return fmt.Errorf("invalid database type: %w", err)
	}
	components := workflow.InitializeSchemaComponents(dbType, goMigVerbose)
	allSchemas, err := workflow.ScanAndParseSchemas(components, goMigVerbose, cfg)
	if err != nil {
		return fmt.Errorf("parsing schema: %w", err)
	}

	// Merge parsed schemas into a single schema for diffing
	currentSchema, err := workflow.MergeAndValidateSchemas(components, allSchemas, dbType, goMigVerbose)
	if err != nil {
		return fmt.Errorf("merging schemas: %w", err)
	}

	// 3. Diff
	diffEngine := yamlpkg.NewDiffEngine(goMigVerbose)
	diff, err := diffEngine.CompareSchemas(prevSchema, currentSchema)
	if err != nil {
		return fmt.Errorf("computing schema diff: %w", err)
	}

	// Prepend a SetDefaults operation when the active DB defaults have changed.
	prevDefaults := getDefaultsForDB(prevSchema, cfg.Database.Type)
	currDefaults := getDefaultsForDB(currentSchema, cfg.Database.Type)
	if !mapsEqual(prevDefaults, currDefaults) && len(currDefaults) > 0 {
		diff.Changes = append([]yamlpkg.Change{{
			Type:        yamlpkg.ChangeTypeDefaultsModified,
			Description: "Update schema defaults",
			OldValue:    prevDefaults,
			NewValue:    currDefaults,
		}}, diff.Changes...)
		diff.HasChanges = true
	}

	// Prepend a SetTypeMappings operation when the active DB type mappings have changed.
	prevMappings := getTypeMappingsForDB(prevSchema, cfg.Database.Type)
	currMappings := getTypeMappingsForDB(currentSchema, cfg.Database.Type)
	if !mapsEqual(prevMappings, currMappings) && len(currMappings) > 0 {
		diff.Changes = append([]yamlpkg.Change{{
			Type:        yamlpkg.ChangeTypeTypeMappingsModified,
			Description: "Update schema type mappings",
			OldValue:    prevMappings,
			NewValue:    currMappings,
		}}, diff.Changes...)
		diff.HasChanges = true
	}

	// 4. Handle merge if requested
	if goMigMerge && dagOut != nil && dagOut.HasUnresolvedBranches {
		return goGenerateMerge(migrationsDir, dagOut, goMigDryRun, goMigVerbose)
	}

	// 5. Check for unresolved branches (warn if present and not doing merge).
	// HasUnresolvedBranches is true only when there are multiple leaf migrations —
	// resolved diamond topologies (already merged) do not trigger this warning.
	// The warning is plain text, so it is suppressed under --json to avoid
	// corrupting the JSON stream on stdout.
	if dagOut != nil && dagOut.HasUnresolvedBranches && !goMigMerge && !goMigJSON {
		fmt.Println("WARNING: Multiple migration branches detected — merge required.")
		for i, leaf := range dagOut.Leaves {
			fmt.Printf("  Branch %d: %s\n", i+1, leaf)
		}
		fmt.Println("Run 'morphic generate --merge' to generate a merge migration.")
	}

	deps := []string{}
	if dagOut != nil {
		deps = dagOut.Leaves
	}

	if !diff.HasChanges {
		if goMigDryRun && goMigJSON {
			// Emit a valid, empty JSON report rather than falling through to the
			// plain-text message so --dry-run --json consumers always get JSON.
			if err := writeDryRunJSON(os.Stdout, "", deps, diff, ""); err != nil {
				return fmt.Errorf("writing JSON report: %w", err)
			}
			return nil
		}
		fmt.Println("No changes detected.")
		return nil
	}

	if goMigCheck {
		printChangeList(diff.Changes)
		return fmt.Errorf("migrations needed: %d changes detected", len(diff.Changes))
	}

	// 6. Determine next migration name
	count := len(migFiles)
	name := BuildMigrationName(count, goMigName, diffEngine.GenerateMigrationName(diff))

	// 7. Prompt for destructive operations and build per-change decisions.
	decisions, err := promptGoMigDecisions(diff, goMigAutoApprove || goMigDryRun)
	if err != nil {
		return err // includes user-requested exit
	}

	// 8. Generate Go source
	src, err := gen.GenerateMigration(name, deps, diff, currentSchema, prevSchema, decisions)
	if err != nil {
		return fmt.Errorf("generating migration source: %w", err)
	}

	if goMigDryRun {
		if goMigJSON {
			if err := writeDryRunJSON(os.Stdout, name, deps, diff, src); err != nil {
				return fmt.Errorf("writing JSON report: %w", err)
			}
		} else {
			printDryRunSummary(name, diff, src)
		}
		if diff.IsDestructive {
			// Return an error so cobra exits with code 1, but use
			// SilenceErrors so the message isn't printed after our output.
			cmd.SilenceErrors = true
			return fmt.Errorf("destructive operations detected")
		}
		return nil
	}

	// 9. Write file
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return fmt.Errorf("creating migrations directory: %w", err)
	}
	outPath := filepath.Join(migrationsDir, name+gen.FileExtension())
	if err := os.WriteFile(outPath, []byte(src), 0o644); err != nil {
		return fmt.Errorf("writing migration file: %w", err)
	}
	fmt.Printf("Created %s\n", outPath)
	return nil
}

// printChangeListTo writes a human-readable summary of schema changes grouped by type.
// When showDestructive is true, groups containing destructive changes are tagged with
// [DESTRUCTIVE].
func printChangeListTo(w io.Writer, changes []yamlpkg.Change, showDestructive bool) {
	type entry struct {
		table string
		field string
		desc  string
	}
	groups := make(map[yamlpkg.ChangeType][]entry)
	destructiveTypes := make(map[yamlpkg.ChangeType]bool)
	for _, c := range changes {
		groups[c.Type] = append(groups[c.Type], entry{c.TableName, c.FieldName, c.Description})
		if c.Destructive {
			destructiveTypes[c.Type] = true
		}
	}

	labels := []struct {
		ct    yamlpkg.ChangeType
		label string
	}{
		{yamlpkg.ChangeTypeTableAdded, "Tables added"},
		{yamlpkg.ChangeTypeTableRemoved, "Tables removed"},
		{yamlpkg.ChangeTypeTableRenamed, "Tables renamed"},
		{yamlpkg.ChangeTypeFieldAdded, "Fields added"},
		{yamlpkg.ChangeTypeFieldRemoved, "Fields removed"},
		{yamlpkg.ChangeTypeFieldRenamed, "Fields renamed"},
		{yamlpkg.ChangeTypeFieldModified, "Fields modified"},
		{yamlpkg.ChangeTypeIndexAdded, "Indexes added"},
		{yamlpkg.ChangeTypeIndexRemoved, "Indexes removed"},
		{yamlpkg.ChangeTypeForeignKeyAdded, "Foreign keys added"},
		{yamlpkg.ChangeTypeForeignKeyRemoved, "Foreign keys removed"},
		{yamlpkg.ChangeTypeDefaultsModified, "Defaults modified"},
		{yamlpkg.ChangeTypeTypeMappingsModified, "Type mappings modified"},
	}

	_, _ = fmt.Fprintln(w)
	for _, l := range labels {
		entries, ok := groups[l.ct]
		if !ok {
			continue
		}
		marker := ""
		if showDestructive && destructiveTypes[l.ct] {
			marker = "  [DESTRUCTIVE]"
		}
		_, _ = fmt.Fprintf(w, "  %s (%d):%s\n", l.label, len(entries), marker)
		for _, e := range entries {
			if e.field != "" {
				_, _ = fmt.Fprintf(w, "    - %s.%s\n", e.table, e.field)
			} else {
				_, _ = fmt.Fprintf(w, "    - %s\n", e.table)
			}
		}
	}
	_, _ = fmt.Fprintln(w)
}

// printChangeList prints a human-readable change summary to stdout without
// destructive markers (used in the normal generation flow).
func printChangeList(changes []yamlpkg.Change) {
	printChangeListTo(os.Stdout, changes, false)
}

// writeDryRunJSON writes the dry-run report as JSON to w.
func writeDryRunJSON(w io.Writer, name string, deps []string, diff *yamlpkg.SchemaDiff, src string) error {
	report := DryRunReport{
		MigrationName:  name,
		Dependencies:   deps,
		HasDestructive: diff.IsDestructive,
		Changes:        make([]DryRunChange, 0, len(diff.Changes)),
		Source:         src,
	}
	if report.Dependencies == nil {
		report.Dependencies = []string{}
	}
	for _, c := range diff.Changes {
		if c.Destructive {
			report.DestructiveCount++
		}
		report.Changes = append(report.Changes, DryRunChange{
			Type:        string(c.Type),
			Table:       c.TableName,
			Field:       c.FieldName,
			Destructive: c.Destructive,
			Description: c.Description,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// printDryRunSummary prints the full human-readable dry-run output: change
// summary with destructive markers, a destructive warning section, and the
// annotated migration source.
func printDryRunSummary(name string, diff *yamlpkg.SchemaDiff, src string) {
	fmt.Printf("Morphic Dry Run: %s\n", name)
	fmt.Printf("\nChanges (%d):\n", len(diff.Changes))
	printChangeListTo(os.Stdout, diff.Changes, true)

	// Destructive warning section.
	var destructive []yamlpkg.Change
	for _, c := range diff.Changes {
		if c.Destructive {
			destructive = append(destructive, c)
		}
	}
	if len(destructive) > 0 {
		fmt.Printf("WARNING: %d destructive operation(s) detected:\n", len(destructive))
		for _, c := range destructive {
			fmt.Printf("  - %s\n", c.Description)
		}
		fmt.Println()
	}

	fmt.Println("--- Migration Source ---")
	fmt.Println(src)
}

// promptGoMigDecisions iterates through diff.Changes and, for each destructive
// operation, shows a bubbletea selector for the user to choose an action and
// scope. The returned map is keyed by change index.
//
// Scope is toggled with Tab: "This only" → "All remaining" → "All of this type".
//
// If the user chooses PromptOmit the generated operation will have SchemaOnly: true
// (schema state advances but no SQL is executed). If the user chooses PromptExit
// an error is returned and migration generation is cancelled.
//
// When autoApprove is true, all destructive operations are automatically approved
// without prompting (for use in CI/CD or non-TTY environments).
func promptGoMigDecisions(diff *yamlpkg.SchemaDiff, autoApprove bool) (map[int]yamlpkg.PromptResponse, error) {
	decisions := make(map[int]yamlpkg.PromptResponse)
	applyAll := yamlpkg.PromptResponse(0)
	applyByType := make(map[yamlpkg.ChangeType]yamlpkg.PromptResponse)

	for i, change := range diff.Changes {
		if !yamlpkg.IsDestructiveOperation(change.Type) {
			continue
		}
		if applyAll != 0 {
			decisions[i] = applyAll
			continue
		}
		if resp, ok := applyByType[change.Type]; ok {
			decisions[i] = resp
			continue
		}

		var title string
		if change.FieldName != "" {
			title = fmt.Sprintf("Destructive: %s on %q (field: %q)", change.Type, change.TableName, change.FieldName)
		} else {
			title = fmt.Sprintf("Destructive: %s on %q", change.Type, change.TableName)
		}

		// In auto-approve mode, generate all destructive operations without prompting.
		if autoApprove {
			fmt.Fprintf(os.Stderr, "Auto-approving: %s\n", title)
			decisions[i] = yamlpkg.PromptGenerate
			continue
		}

		resp, scope, err := ui.RunDestructivePrompt(title, change.Type)
		if err != nil {
			return nil, err
		}
		if resp == yamlpkg.PromptExit {
			return nil, fmt.Errorf("migration generation cancelled by user")
		}

		decisions[i] = resp
		switch scope {
		case ui.ScopeAll:
			applyAll = resp
		case ui.ScopeType:
			applyByType[change.Type] = resp
		}
	}
	return decisions, nil
}

// queryDAG loads the migrations directory and returns the current migration
// graph state. Migration .star files are interpreted in-process and registered
// with a fresh *migrate.Registry, then BuildGraph + ToDAGOutput run directly.
//
// The verbose parameter is preserved for API compatibility with callers but
// has no effect now that no build step takes place.
func queryDAG(migrationsDir string, _ bool) (*migrate.DAGOutput, error) {
	reg, err := interp.LoadRegistry(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("loading migrations: %w", err)
	}
	g, err := migrate.BuildGraph(reg)
	if err != nil {
		return nil, fmt.Errorf("building migration graph: %w", err)
	}
	return g.ToDAGOutput()
}

// schemaStateToYAMLSchema converts a migrate.SchemaState (reconstructed from
// the migration DAG) into a yaml.Schema suitable for diffing against the
// current YAML schema. Tables are sorted by name for deterministic output.
// dbType is used to populate the Defaults section of the schema so that
// defaults changes are detected on subsequent diff runs.
func schemaStateToYAMLSchema(state *migrate.SchemaState, dbType string) *yamlpkg.Schema {
	if state == nil {
		return nil
	}
	schema := &yamlpkg.Schema{}
	for _, ts := range state.Tables {
		t := yamlpkg.Table{Name: ts.Name}
		for _, f := range ts.Fields {
			nullable := f.Nullable
			yf := yamlpkg.Field{
				Name:       f.Name,
				Type:       f.Type,
				PrimaryKey: f.PrimaryKey,
				Nullable:   &nullable,
				Default:    f.Default,
				Length:     f.Length,
				Precision:  f.Precision,
				Scale:      f.Scale,
				AutoCreate: f.AutoCreate,
				AutoUpdate: f.AutoUpdate,
			}
			if f.ForeignKey != nil {
				// Only include the FK annotation when the constraint actually exists in
				// the migration state. If it is absent (e.g. tables created before
				// AddForeignKey support was introduced), leave ForeignKey nil so the
				// diff engine detects the missing constraint and emits
				// ChangeTypeForeignKeyAdded.
				// Compute the constraint name through the same SafeConstraintName
				// helper the generator/writer uses (internal/codegen), so long
				// names (>63 chars) that the writer truncates+hashes match here on
				// reconstruction. Without this, a raw name never equals the stored
				// truncated name and the FK is perpetually re-emitted as "added".
				constraintName := utils.SafeConstraintName(fmt.Sprintf("fk_%s_%s", ts.Name, f.Name))
				for _, fkc := range ts.ForeignKeys {
					if fkc.Name == constraintName {
						yf.ForeignKey = &yamlpkg.ForeignKey{
							Table:    fkc.ReferencedTable,
							OnDelete: fkc.OnDelete,
						}
						break
					}
				}
			}
			t.Fields = append(t.Fields, yf)
		}
		for _, idx := range ts.Indexes {
			if idx.FromFK {
				continue
			}
			t.Indexes = append(t.Indexes, yamlpkg.Index{
				Name:   idx.Name,
				Fields: idx.Fields,
				Unique: idx.Unique,
				Method: idx.Method,
				Where:  idx.Where,
			})
		}
		schema.Tables = append(schema.Tables, t)
	}
	// Sort tables for determinism
	sort.Slice(schema.Tables, func(i, j int) bool {
		return schema.Tables[i].Name < schema.Tables[j].Name
	})
	// Populate the Defaults section so that defaults changes are detected on
	// subsequent diff runs (the diff engine compares schema.Defaults).
	if len(state.Defaults) > 0 {
		if schema.Defaults == nil {
			schema.Defaults = make(yamlpkg.Defaults)
		}
		schema.Defaults.SetForProvider(types.DatabaseType(dbType), state.Defaults)
	}
	// Populate TypeMappings so that type mapping changes are detected on subsequent diff runs.
	if len(state.TypeMappings) > 0 {
		if schema.TypeMappings == nil {
			schema.TypeMappings = make(yamlpkg.TypeMappings)
		}
		schema.TypeMappings.SetForProvider(types.DatabaseType(dbType), state.TypeMappings)
	}
	return schema
}

// getDefaultsForDB returns the defaults map for the given database type from a schema.
// Returns nil when the schema or the relevant DB defaults are empty.
func getDefaultsForDB(schema *yamlpkg.Schema, dbType string) map[string]string {
	if schema == nil {
		return nil
	}
	return schema.Defaults.ForProvider(types.DatabaseType(dbType))
}

// getTypeMappingsForDB returns the type mappings for the given database type from a schema.
// Returns nil when the schema or the relevant DB type mappings are empty.
func getTypeMappingsForDB(schema *yamlpkg.Schema, dbType string) map[string]string {
	if schema == nil {
		return nil
	}
	return schema.TypeMappings.ForProvider(types.DatabaseType(dbType))
}

// mapsEqual reports whether two map[string]string values are identical.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// BuildMigrationName builds the migration name from a sequence number and an
// optional custom name. If customName is provided, it is normalized (lowered,
// spaces replaced with underscores). Otherwise autoName (from the diff engine)
// is used. As a final fallback a timestamp is appended.
// Exported for testing.
func BuildMigrationName(currentCount int, customName, autoName string) string {
	num := codegen.NextMigrationNumber(currentCount)
	if customName != "" {
		return fmt.Sprintf("%s_%s", num, strings.ToLower(strings.ReplaceAll(customName, " ", "_")))
	}
	if autoName != "" {
		return fmt.Sprintf("%s_%s", num, autoName)
	}
	return fmt.Sprintf("%s_%s", num, time.Now().Format("20060102150405"))
}

// newGenerator returns the Starlark MigrationGenerator.
func newGenerator(_ *config.Config) codegen.MigrationGenerator {
	return codegen.NewStarlarkGenerator()
}

// countMigrationFiles returns all migration files (.go and .star) in the directory,
// excluding main.go and test files.
func countMigrationFiles(migrationsDir string) ([]string, error) {
	goFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.go"))
	starFiles, _ := filepath.Glob(filepath.Join(migrationsDir, "*.star"))

	var migFiles []string
	for _, f := range goFiles {
		base := filepath.Base(f)
		if base != "main.go" && !strings.HasSuffix(base, "_test.go") {
			migFiles = append(migFiles, f)
		}
	}
	migFiles = append(migFiles, starFiles...)
	sort.Strings(migFiles)
	return migFiles, nil
}

// goGenerateMerge generates a merge migration for detected branches. It uses
// codegen.MergeGenerator to produce a Starlark file that depends on all branch
// leaves but contains no operations, thus unifying the DAG.
func goGenerateMerge(migrationsDir string, dagOut *migrate.DAGOutput, dryRun, verbose bool) error {
	// Count existing migration files
	allMigFiles, _ := countMigrationFiles(migrationsDir)
	count := len(allMigFiles)

	name := fmt.Sprintf("%s_merge_%s", codegen.NextMigrationNumber(count),
		strings.Join(dagOut.Leaves, "_and_"))
	// Truncate if too long
	if len(name) > 80 {
		name = fmt.Sprintf("%s_merge", codegen.NextMigrationNumber(count))
	}

	mergeGen := codegen.NewMergeGenerator()
	cfg := config.LoadOrDefault(cfgFile)
	format := codegen.ParseMigrationFormat(cfg.Migration.Format)

	src, err := mergeGen.GenerateStarlarkMerge(name, dagOut.Leaves)
	if err != nil {
		return fmt.Errorf("generating merge migration: %w", err)
	}

	if dryRun {
		fmt.Println(src)
		return nil
	}

	if verbose {
		fmt.Printf("Generating merge migration: %s\n", name)
	}

	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return fmt.Errorf("creating migrations directory: %w", err)
	}
	outPath := filepath.Join(migrationsDir, codegen.MigrationFileNameForFormat(name, format))
	if err := os.WriteFile(outPath, []byte(src), 0o644); err != nil {
		return fmt.Errorf("writing merge migration: %w", err)
	}
	fmt.Printf("Created merge migration: %s\n", outPath)
	fmt.Printf("Dependencies: %s\n", strings.Join(dagOut.Leaves, ", "))
	return nil
}
