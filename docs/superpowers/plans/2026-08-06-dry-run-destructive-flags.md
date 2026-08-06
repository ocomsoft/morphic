# Enhanced --dry-run with Destructive Flagging — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `morphic generate --dry-run` output a human-readable change summary with destructive operations flagged, inject `# DESTRUCTIVE` comments into the migration source, support `--json` for machine-readable output, and exit with code 1 when destructive ops are present.

**Architecture:** Enhance the existing `printChangeList` in `cmd/generate.go` to support destructive markers. Add a new `printDryRunSummary` wrapper that prints the destructive warning section + annotated source. Add `# DESTRUCTIVE` comment injection in `StarlarkGenerator.GenerateMigration`. Add a `--json` flag that emits a structured JSON report. Exit code logic gates on `diff.IsDestructive`.

**Tech Stack:** Go, Cobra (CLI flags), `encoding/json` (JSON output)

## Global Constraints

- All new code must pass `golangci-lint run -v ./...` with zero issues.
- All new code must have tests.
- Each change is committed individually.
- `--json` is only valid with `--dry-run`; error if used alone.
- Exit code 1 for destructive ops applies to both human and JSON output modes.

---

### Task 1: Inject `# DESTRUCTIVE` comments in StarlarkGenerator

**Files:**
- Modify: `internal/codegen/starlark_generator.go:85-100` (the `GenerateMigration` loop)
- Test: `internal/codegen/starlark_generator_test.go`

**Interfaces:**
- Consumes: `yaml.Change.Destructive` bool field (already exists)
- Produces: `# DESTRUCTIVE: <description>` comment lines in generated `.star` source above each destructive operation

- [ ] **Step 1: Write the failing test**

In `internal/codegen/starlark_generator_test.go`, add:

```go
func TestStarlarkGenerator_DestructiveComment_DropTable(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:        yaml.ChangeTypeTableRemoved,
				TableName:   "old_table",
				Destructive: true,
				Description: "Remove table 'old_table'",
			},
		},
	}

	src, err := gen.GenerateMigration("0003_drop", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `# DESTRUCTIVE: Remove table 'old_table'`)
	assertContains(t, src, `drop_table("old_table")`)
}

func TestStarlarkGenerator_DestructiveComment_DropField(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:        yaml.ChangeTypeFieldRemoved,
				TableName:   "users",
				FieldName:   "legacy_col",
				Destructive: true,
				Description: "Remove field 'users.legacy_col'",
			},
		},
	}

	src, err := gen.GenerateMigration("0004_drop_col", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	assertContains(t, src, `# DESTRUCTIVE: Remove field 'users.legacy_col'`)
	assertContains(t, src, `drop_field("users", "legacy_col")`)
}

func TestStarlarkGenerator_NoDestructiveComment_NonDestructive(t *testing.T) {
	gen := codegen.NewStarlarkGenerator()
	diff := &yaml.SchemaDiff{
		HasChanges: true,
		Changes: []yaml.Change{
			{
				Type:        yaml.ChangeTypeFieldAdded,
				TableName:   "users",
				FieldName:   "phone",
				Destructive: false,
				NewValue:    yaml.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: boolPtr(true)},
			},
		},
	}

	src, err := gen.GenerateMigration("0005_add_phone", nil, diff, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateMigration: %v", err)
	}
	if strings.Contains(src, "# DESTRUCTIVE") {
		t.Errorf("non-destructive operation should not have DESTRUCTIVE comment")
	}
}
```

Note: `boolPtr` and `assertContains` are test helpers already defined in the test file. Add `"strings"` to the import block if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/codegen/ -run "TestStarlarkGenerator_DestructiveComment|TestStarlarkGenerator_NoDestructiveComment" -v`
Expected: FAIL — no `# DESTRUCTIVE` comment in output.

- [ ] **Step 3: Implement the comment injection**

In `internal/codegen/starlark_generator.go`, in the `GenerateMigration` method, add the destructive comment inside the `for i, change := range diff.Changes` loop, just before the existing `if review {` block (around line 96):

```go
		if change.Destructive {
			fmt.Fprintf(&b, "        # DESTRUCTIVE: %s\n", change.Description)
		}
```

The full loop body (lines 85-100) should now read:

```go
	for i, change := range diff.Changes {
		decision := decisions[i]
		schemaOnly := decision == yaml.PromptOmit
		review := decision == yaml.PromptReview
		ignoreErrors := decision == yaml.PromptIgnoreErrors

		op, err := g.generateOperation(change, currentSchema, previousSchema, schemaOnly, ignoreErrors)
		if err != nil {
			return "", fmt.Errorf("generating operation for change %s on %s: %w",
				change.Type, change.TableName, err)
		}
		if change.Destructive {
			fmt.Fprintf(&b, "        # DESTRUCTIVE: %s\n", change.Description)
		}
		if review {
			b.WriteString("        # REVIEW: destructive operation — verify before running\n")
		}
		b.WriteString(op)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/codegen/ -run "TestStarlarkGenerator_DestructiveComment|TestStarlarkGenerator_NoDestructiveComment" -v`
Expected: PASS

- [ ] **Step 5: Run linter**

Run: `golangci-lint run -v ./internal/codegen/`
Expected: 0 issues

- [ ] **Step 6: Commit**

```bash
git add internal/codegen/starlark_generator.go internal/codegen/starlark_generator_test.go
git commit -m "feat(codegen): inject # DESTRUCTIVE comments for destructive operations"
```

---

### Task 2: Add `--json` flag and enhanced `--dry-run` output in `cmd/generate.go`

**Files:**
- Modify: `cmd/generate.go` (flag registration, dry-run output path, new functions)
- Test: `cmd/generate_test.go`

**Interfaces:**
- Consumes: `yaml.SchemaDiff` (`.Changes`, `.IsDestructive`), `yaml.Change` (`.Destructive`, `.Type`, `.TableName`, `.FieldName`, `.Description`), generated source string from `gen.GenerateMigration`
- Produces:
  - `printChangeList(changes []yaml.Change, showDestructive bool)` — enhanced version with optional `[DESTRUCTIVE]` markers
  - `printDryRunSummary(name string, diff *yaml.SchemaDiff, src string)` — full dry-run human output
  - `printDryRunJSON(name string, deps []string, diff *yaml.SchemaDiff, src string) error` — JSON output
  - `DryRunReport` struct — JSON serialization target
  - `DryRunChange` struct — per-change JSON entry

- [ ] **Step 1: Write the failing tests**

In `cmd/generate_test.go`, add these tests. They test the new helper functions directly (no end-to-end CLI invocation needed since the functions are package-level):

```go
func TestPrintChangeList_WithDestructiveMarkers(t *testing.T) {
	changes := []yamlpkg.Change{
		{Type: yamlpkg.ChangeTypeTableAdded, TableName: "users"},
		{Type: yamlpkg.ChangeTypeTableRemoved, TableName: "posts", Destructive: true},
		{Type: yamlpkg.ChangeTypeFieldRemoved, TableName: "users", FieldName: "old_col", Destructive: true},
	}

	var buf bytes.Buffer
	printChangeListTo(&buf, changes, true)
	out := buf.String()

	if !strings.Contains(out, "[DESTRUCTIVE]") {
		t.Error("expected [DESTRUCTIVE] marker in output")
	}
	if !strings.Contains(out, "Tables added") {
		t.Error("expected 'Tables added' in output")
	}
	if !strings.Contains(out, "Tables removed") {
		t.Error("expected 'Tables removed' in output")
	}
}

func TestPrintChangeList_WithoutDestructiveMarkers(t *testing.T) {
	changes := []yamlpkg.Change{
		{Type: yamlpkg.ChangeTypeTableRemoved, TableName: "posts", Destructive: true},
	}

	var buf bytes.Buffer
	printChangeListTo(&buf, changes, false)
	out := buf.String()

	if strings.Contains(out, "[DESTRUCTIVE]") {
		t.Error("showDestructive=false should not include [DESTRUCTIVE] marker")
	}
}

func TestDryRunJSON_Structure(t *testing.T) {
	diff := &yamlpkg.SchemaDiff{
		HasChanges:    true,
		IsDestructive: true,
		Changes: []yamlpkg.Change{
			{Type: yamlpkg.ChangeTypeTableRemoved, TableName: "posts", Destructive: true, Description: "Remove table 'posts'"},
			{Type: yamlpkg.ChangeTypeFieldAdded, TableName: "users", FieldName: "phone", Destructive: false, Description: "Add field 'users.phone'"},
		},
	}

	var buf bytes.Buffer
	err := writeDryRunJSON(&buf, "0003_remove_posts", []string{"0002_initial"}, diff, "migration(...)")
	if err != nil {
		t.Fatalf("writeDryRunJSON: %v", err)
	}

	var report DryRunReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	if report.MigrationName != "0003_remove_posts" {
		t.Errorf("expected migration_name '0003_remove_posts', got %q", report.MigrationName)
	}
	if !report.HasDestructive {
		t.Error("expected has_destructive=true")
	}
	if report.DestructiveCount != 1 {
		t.Errorf("expected destructive_count=1, got %d", report.DestructiveCount)
	}
	if len(report.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(report.Changes))
	}
	if !report.Changes[0].Destructive {
		t.Error("first change should be destructive")
	}
	if report.Changes[1].Destructive {
		t.Error("second change should not be destructive")
	}
	if report.Source != "migration(...)" {
		t.Errorf("expected source 'migration(...)', got %q", report.Source)
	}
}
```

Add `"bytes"`, `"encoding/json"`, and `"strings"` to the import block if not already present.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/ -run "TestPrintChangeList_|TestDryRunJSON_" -v`
Expected: FAIL — functions don't exist yet.

- [ ] **Step 3: Register the `--json` flag**

In `cmd/generate.go`, add `goMigJSON bool` to the flag variables block:

```go
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
```

In the `init()` function, add the flag registration after the `--auto-approve` flag:

```go
	goMigrationsCmd.Flags().BoolVar(&goMigJSON, "json", false,
		"Output dry-run results as JSON (requires --dry-run)")
```

- [ ] **Step 4: Add the DryRunReport and DryRunChange structs**

Add these structs after the flag variables in `cmd/generate.go`:

```go
// DryRunReport is the JSON structure emitted by --dry-run --json.
type DryRunReport struct {
	MigrationName    string           `json:"migration_name"`
	Dependencies     []string         `json:"dependencies"`
	HasDestructive   bool             `json:"has_destructive"`
	DestructiveCount int              `json:"destructive_count"`
	Changes          []DryRunChange   `json:"changes"`
	Source           string           `json:"source"`
}

// DryRunChange is a single change entry in the dry-run JSON report.
type DryRunChange struct {
	Type        string `json:"type"`
	Table       string `json:"table"`
	Field       string `json:"field,omitempty"`
	Destructive bool   `json:"destructive"`
	Description string `json:"description"`
}
```

- [ ] **Step 5: Implement `printChangeListTo`**

Refactor the existing `printChangeList` to write to an `io.Writer` with a `showDestructive` flag. Replace the existing function with:

```go
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

	fmt.Fprintln(w)
	for _, l := range labels {
		entries, ok := groups[l.ct]
		if !ok {
			continue
		}
		marker := ""
		if showDestructive && destructiveTypes[l.ct] {
			marker = "  [DESTRUCTIVE]"
		}
		fmt.Fprintf(w, "  %s (%d):%s\n", l.label, len(entries), marker)
		for _, e := range entries {
			if e.field != "" {
				fmt.Fprintf(w, "    - %s.%s\n", e.table, e.field)
			} else {
				fmt.Fprintf(w, "    - %s\n", e.table)
			}
		}
	}
	fmt.Fprintln(w)
}

// printChangeList prints a human-readable change summary to stdout without
// destructive markers (used in the normal generation flow).
func printChangeList(changes []yamlpkg.Change) {
	printChangeListTo(os.Stdout, changes, false)
}
```

Add `"io"` to the import block.

- [ ] **Step 6: Implement `writeDryRunJSON`**

Add this function in `cmd/generate.go`:

```go
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
```

Add `"encoding/json"` to the import block.

- [ ] **Step 7: Implement `printDryRunSummary`**

Add this function in `cmd/generate.go`:

```go
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
```

- [ ] **Step 8: Rewrite the dry-run path in `runGoMakeMigrations`**

Replace the existing dry-run block (currently around lines 232-235):

```go
	if goMigDryRun {
		fmt.Println(src)
		return nil
	}
```

With:

```go
	if goMigDryRun {
		if goMigJSON {
			if err := writeDryRunJSON(os.Stdout, name, deps, diff, src); err != nil {
				return fmt.Errorf("writing JSON report: %w", err)
			}
		} else {
			printDryRunSummary(name, diff, src)
		}
		if diff.IsDestructive {
			os.Exit(1)
		}
		return nil
	}
```

- [ ] **Step 9: Add `--json` without `--dry-run` validation**

At the top of `runGoMakeMigrations`, right after `cfg := config.LoadOrDefault(cfgFile)`, add:

```go
	if goMigJSON && !goMigDryRun {
		return fmt.Errorf("--json requires --dry-run")
	}
```

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./cmd/ -run "TestPrintChangeList_|TestDryRunJSON_" -v`
Expected: PASS

- [ ] **Step 11: Run full test suite for affected packages**

Run: `go test ./cmd/ ./internal/codegen/`
Expected: PASS

- [ ] **Step 12: Run linter**

Run: `golangci-lint run -v ./cmd/`
Expected: 0 issues

- [ ] **Step 13: Commit**

```bash
git add cmd/generate.go cmd/generate_test.go
git commit -m "feat(generate): enhanced --dry-run with destructive flags and --json output"
```

---

### Task 3: Update documentation

**Files:**
- Modify: `docs/commands/generate.md`

**Interfaces:**
- Consumes: all behaviour implemented in Tasks 1-2
- Produces: updated docs reflecting new `--dry-run` behaviour, `--json` flag, `--auto-approve` flag, exit codes

- [ ] **Step 1: Update the Command Flags table**

In `docs/commands/generate.md`, replace the Command Flags table with:

```markdown
| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--auto-approve` | bool | `false` | Automatically approve all destructive operations without prompting (for CI/non-TTY environments) |
| `--check` | bool | `false` | Exit with error code 1 if migrations are needed (CI/CD mode) |
| `--dry-run` | bool | `false` | Print change summary and annotated migration source without writing a file; exits with code 1 if destructive operations are detected |
| `--json` | bool | `false` | Output dry-run results as structured JSON (requires `--dry-run`) |
| `--merge` | bool | `false` | Generate a merge migration for detected concurrent branches |
| `--name` | string | auto-generated | Custom name suffix for the migration file |
| `--verbose` | bool | `false` | Show detailed pipeline output |
```

- [ ] **Step 2: Replace the Dry Run example section**

Replace the existing "### Dry Run" section (around lines 332-353) with:

````markdown
### Dry Run

Preview what the migration will do without writing a file. The output includes a
change summary with destructive operations flagged, followed by the annotated
migration source:

```bash
morphic generate --dry-run
```

```
Morphic Dry Run: 0002_remove_sessions

Changes (3):

  Tables removed (1):  [DESTRUCTIVE]
    - sessions
  Fields added (1):
    - users.phone
  Fields removed (1):  [DESTRUCTIVE]
    - users.old_email

WARNING: 2 destructive operation(s) detected:
  - Remove table 'sessions'
  - Remove field 'users.old_email'

--- Migration Source ---
migration(
    name = "0002_remove_sessions",
    dependencies = ["0001_initial"],
    operations = [
        # DESTRUCTIVE: Remove table 'sessions'
        drop_table("sessions"),
        add_field("users", varchar("phone", 20, nullable = True)),
        # DESTRUCTIVE: Remove field 'users.old_email'
        drop_field("users", "old_email"),
    ],
)
```

**Exit codes:** `--dry-run` exits with code **0** when no destructive operations
are present, and code **1** when destructive operations are detected. This
allows CI pipelines to gate on destructive changes without parsing the output.

### Dry Run with JSON Output

For machine-readable output (useful for AI agents and automation):

```bash
morphic generate --dry-run --json
```

```json
{
  "migration_name": "0002_remove_sessions",
  "dependencies": ["0001_initial"],
  "has_destructive": true,
  "destructive_count": 2,
  "changes": [
    {
      "type": "table_removed",
      "table": "sessions",
      "destructive": true,
      "description": "Remove table 'sessions'"
    },
    {
      "type": "field_added",
      "table": "users",
      "field": "phone",
      "destructive": false,
      "description": "Add field 'users.phone'"
    },
    {
      "type": "field_removed",
      "table": "users",
      "field": "old_email",
      "destructive": true,
      "description": "Remove field 'users.old_email'"
    }
  ],
  "source": "migration(\n    name = \"0002_remove_sessions\",\n    ..."
}
```

The same exit code rules apply: code 1 if `has_destructive` is true.
````

- [ ] **Step 3: Update the "Skipping the Prompt" section**

Replace the "Skipping the Prompt" section with:

```markdown
### Skipping the Prompt

Use `--auto-approve` to auto-accept all destructive operations as **Generate** without prompting:

\```bash
morphic generate --auto-approve
\```

This is equivalent to always choosing option 1. Useful in CI/CD or non-interactive
environments. Combine with `--dry-run` to preview what would be auto-approved:

\```bash
morphic generate --dry-run --auto-approve
\```
```

- [ ] **Step 4: Update CI/CD section**

In the CI/CD section, add a new example after the existing GitHub Actions block:

````markdown
### Gating on Destructive Changes

Use `--dry-run` in CI to block PRs that introduce destructive migrations:

```yaml
# .github/workflows/check-destructive.yml
name: Check Destructive Migrations
on: [pull_request]

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Install morphic
        run: go install github.com/ocomsoft/morphic@latest
      - name: Check for destructive changes
        run: morphic generate --dry-run --auto-approve
      - name: Get migration details (on failure)
        if: failure()
        run: morphic generate --dry-run --json --auto-approve
```

### AI Agent Integration

Use `--dry-run --json` for AI agents that need to understand migration impact:

```bash
# Agent reads the JSON and decides whether to proceed
REPORT=$(morphic generate --dry-run --json --auto-approve 2>/dev/null)
if [ $? -eq 1 ]; then
    echo "Destructive migration detected — requesting human review"
    echo "$REPORT" | jq '.changes[] | select(.destructive)'
fi
```
````

- [ ] **Step 5: Verify docs render correctly**

Read through the updated file to check for formatting issues, broken markdown, or stale references (e.g. `--silent` should now be `--auto-approve`).

- [ ] **Step 6: Commit**

```bash
git add docs/commands/generate.md
git commit -m "docs(generate): document --dry-run destructive flags, --json, and --auto-approve"
```
