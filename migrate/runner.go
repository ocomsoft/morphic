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

package migrate

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ocomsoft/morphic/internal/providers"
)

// RunOptions controls optional behaviour for Up and Down.
type RunOptions struct {
	// WarnOnMissingDrop causes drop operations that fail because the target
	// object does not exist to print a warning and continue rather than stop.
	WarnOnMissingDrop bool
	// FakeInitial causes migrations marked Initial=true to be faked (recorded
	// as applied without running SQL) when all their CreateTable tables and
	// columns already exist in the live database.
	FakeInitial bool
}

// Runner executes migrations against a database in topological order.
type Runner struct {
	graph         *Graph
	provider      providers.Provider
	db            *sql.DB
	recorder      *MigrationRecorder
	output        io.Writer
	migrationsDir string
}

// NewRunner creates a Runner using the given graph, provider, db, recorder,
// output writer, and migrationsDir. If output is nil, os.Stdout is used.
// migrationsDir is passed to operations so they can resolve external data files.
func NewRunner(graph *Graph, provider providers.Provider, db *sql.DB, recorder *MigrationRecorder, output io.Writer, migrationsDir string) *Runner {
	if output == nil {
		output = os.Stdout
	}
	return &Runner{
		graph:         graph,
		provider:      provider,
		db:            db,
		recorder:      recorder,
		output:        output,
		migrationsDir: migrationsDir,
	}
}

// printf writes formatted output to the runner's output writer.
// Errors from writing are intentionally discarded since these are
// informational messages (progress, warnings) and write failures
// should not abort migrations.
func (r *Runner) printf(format string, a ...interface{}) {
	_, _ = fmt.Fprintf(r.output, format, a...)
}

// Up applies all pending migrations in topological order.
// If to is non-empty, stops after applying the named migration.
func (r *Runner) Up(to string, opts RunOptions) error {
	plan, err := r.graph.Linearize()
	if err != nil {
		return fmt.Errorf("linearizing graph: %w", err)
	}
	applied, err := r.recorder.GetApplied()
	if err != nil {
		return fmt.Errorf("getting applied migrations: %w", err)
	}
	state := NewSchemaState()

	// Replay already-applied migrations to rebuild state
	for _, mig := range plan {
		if !applied[mig.Name] {
			continue
		}
		for _, op := range mig.Operations {
			if err := op.Mutate(state); err != nil {
				return fmt.Errorf("replaying state for %q: %w", mig.Name, err)
			}
		}
	}

	for _, mig := range plan {
		if applied[mig.Name] {
			continue
		}

		// Check if this initial migration can be faked.
		if opts.FakeInitial && mig.Initial {
			canFake, err := r.shouldFakeInitial(mig)
			if err != nil {
				return fmt.Errorf("checking fake-initial for %q: %w", mig.Name, err)
			}
			if canFake {
				// Replay state mutations first so subsequent migrations see the schema.
				// Done before recording to maintain atomicity — if Mutate fails,
				// the migration is not recorded as applied.
				for _, op := range mig.Operations {
					if err := op.Mutate(state); err != nil {
						return fmt.Errorf("replaying state for faked %q: %w", mig.Name, err)
					}
				}
				if err := r.recorder.RecordApplied(mig.Name); err != nil {
					return fmt.Errorf("recording faked migration %q: %w", mig.Name, err)
				}
				r.printf("Applying %s... faked (tables already exist)\n", mig.Name)
				if to != "" && mig.Name == to {
					break
				}
				continue
			}
		}

		r.printf("Applying %s...", mig.Name)
		if err := r.applyMigration(mig, state, opts); err != nil {
			r.printf(" FAILED\n")
			return fmt.Errorf("applying migration %q: %w", mig.Name, err)
		}
		r.printf(" done\n")
		if to != "" && mig.Name == to {
			break
		}
	}
	return nil
}

// extractExpectedTables walks a migration's operations and returns a map of
// table name → expected column names for all CreateTable operations.
// If the migration contains any non-CreateTable operations, it returns nil:
// such migrations do more than just create tables, so faking them based on
// table existence alone could silently skip real DDL (e.g. AddField ops
// folded in from a squash).
func extractExpectedTables(mig *Migration) map[string][]string {
	tables := make(map[string][]string)
	for _, op := range mig.Operations {
		ct, ok := op.(*CreateTable)
		if !ok {
			// Non-CreateTable operations mean this migration does more than
			// just create tables — not safe to fake based on table existence alone.
			return nil
		}
		var cols []string
		for _, f := range ct.Fields {
			cols = append(cols, f.Name)
		}
		tables[ct.Name] = cols
	}
	return tables
}

// shouldFakeInitial checks whether an initial migration can be faked by
// verifying that all its CreateTable tables exist in the live database with
// the expected columns present. Extra columns in the live DB are allowed.
func (r *Runner) shouldFakeInitial(mig *Migration) (bool, error) {
	expected := extractExpectedTables(mig)
	if len(expected) == 0 {
		return false, nil
	}

	for tableName, expectedCols := range expected {
		liveCols, err := r.provider.TableColumns(r.db, tableName)
		if err != nil {
			return false, fmt.Errorf("checking table %q: %w", tableName, err)
		}
		if liveCols == nil {
			return false, nil
		}

		liveSet := make(map[string]bool, len(liveCols))
		for _, c := range liveCols {
			liveSet[c] = true
		}

		for _, col := range expectedCols {
			if !liveSet[col] {
				return false, nil
			}
		}
	}

	return true, nil
}

// Down rolls back migrations. If steps > 0, rolls back that many.
// If to is set, rolls back until that migration name is reached (exclusive).
func (r *Runner) Down(steps int, to string, opts RunOptions) error {
	plan, err := r.graph.Linearize()
	if err != nil {
		return fmt.Errorf("linearizing graph: %w", err)
	}
	applied, err := r.recorder.GetApplied()
	if err != nil {
		return fmt.Errorf("getting applied migrations: %w", err)
	}

	// Collect applied migrations in reverse topological order
	var toRollback []*Migration
	for i := len(plan) - 1; i >= 0; i-- {
		if applied[plan[i].Name] {
			toRollback = append(toRollback, plan[i])
		}
	}

	for i, mig := range toRollback {
		if steps > 0 && i >= steps {
			break
		}
		if to != "" && mig.Name == to {
			break
		}
		// Reconstruct state just before this migration by replaying
		// all applied migrations up to (but not including) this one
		state := NewSchemaState()
		for _, m := range plan {
			if m.Name == mig.Name {
				break
			}
			if applied[m.Name] {
				for _, op := range m.Operations {
					if err := op.Mutate(state); err != nil {
						return fmt.Errorf("replaying state for %q: %w", m.Name, err)
					}
				}
			}
		}
		r.printf("Rolling back %s...", mig.Name)
		if err := r.rollbackMigration(mig, state, opts); err != nil {
			r.printf(" FAILED\n")
			return fmt.Errorf("rolling back migration %q: %w", mig.Name, err)
		}
		r.printf(" done\n")
	}
	return nil
}

// Status prints migration status: applied vs pending.
func (r *Runner) Status() error {
	plan, err := r.graph.Linearize()
	if err != nil {
		return err
	}
	applied, err := r.recorder.GetApplied()
	if err != nil {
		return err
	}
	r.printf("%-50s %s\n", "Migration", "Status")
	r.printf("%s\n", strings.Repeat("-", 60))
	for _, mig := range plan {
		status := "Pending"
		if applied[mig.Name] {
			status = "Applied"
		}
		r.printf("%-50s %s\n", mig.Name, status)
	}
	return nil
}

// ShowSQL prints all pending migration SQL without executing it.
func (r *Runner) ShowSQL() error {
	plan, err := r.graph.Linearize()
	if err != nil {
		return err
	}
	applied, err := r.recorder.GetApplied()
	if err != nil {
		return err
	}
	state := NewSchemaState()
	for _, mig := range plan {
		if applied[mig.Name] {
			for _, op := range mig.Operations {
				if err := op.Mutate(state); err != nil {
					return fmt.Errorf("replaying state for %q: %w", mig.Name, err)
				}
			}
			continue
		}
		r.printf("-- %s\n", mig.Name)
		for i, op := range mig.Operations {
			r.provider.SetTypeMappings(state.TypeMappings)
			sqlStr, err := op.Up(r.provider, state, state.Defaults, r.migrationsDir)
			if err != nil {
				return fmt.Errorf("%s operation %d/%d [%s]: %w", mig.Name, i+1, len(mig.Operations), op.Describe(), err)
			}
			if sqlStr != "" {
				r.printf("%s\n\n", sqlStr)
			}
			if err := op.Mutate(state); err != nil {
				return fmt.Errorf("%s operation %d/%d [%s]: mutating state: %w", mig.Name, i+1, len(mig.Operations), op.Describe(), err)
			}
		}
	}
	return nil
}

// applyMigration executes all operations in a migration within a transaction
// and records it as applied atomically. If any operation fails the transaction
// is rolled back and the database is left unchanged.
// When opts.WarnOnMissingDrop is true, drop operations that fail because the object
// does not exist are skipped with a warning instead of stopping the migration.
//
// Note: DDL statements in MySQL are auto-committed and cannot be rolled back
// regardless of the transaction. PostgreSQL supports transactional DDL fully.
func (r *Runner) applyMigration(mig *Migration, state *SchemaState, opts RunOptions) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if already committed

	for i, op := range mig.Operations {
		r.provider.SetTypeMappings(state.TypeMappings)
		sqlStr, err := op.Up(r.provider, state, state.Defaults, r.migrationsDir)
		if err != nil {
			return fmt.Errorf("operation %d/%d [%s]: generating SQL: %w", i+1, len(mig.Operations), op.Describe(), err)
		}
		skipped := false
		if sqlStr != "" {
			if execErr := execWithSavepoint(tx, sqlStr, canIgnoreError(op, opts)); execErr != nil {
				if shouldIgnoreError(op, opts, r.provider, execErr) {
					r.printf("[WARNING] op %d/%d %s — %v, skipping\n", i+1, len(mig.Operations), op.Describe(), execErr)
					skipped = true
				} else {
					return fmt.Errorf("operation %d/%d [%s]: %w\n  SQL: %s", i+1, len(mig.Operations), op.Describe(), execErr, sqlStr)
				}
			}
		}
		// Skip state mutation when the drop operation was skipped — the object
		// was never in the schema state either, so Mutate would fail.
		if !skipped {
			if err := op.Mutate(state); err != nil {
				return fmt.Errorf("operation %d/%d [%s]: mutating state: %w", i+1, len(mig.Operations), op.Describe(), err)
			}
		}
	}

	if err := r.recorder.RecordAppliedTx(tx, mig.Name); err != nil {
		return err
	}

	return tx.Commit()
}

// rollbackMigration reverses all operations in a migration within a transaction
// and removes it from history atomically. If any operation fails the transaction
// is rolled back and the database is left unchanged.
// When opts.WarnOnMissingDrop is true, drop operations that fail because the object
// does not exist are skipped with a warning instead of stopping the rollback.
//
// Note: DDL statements in MySQL are auto-committed and cannot be rolled back
// regardless of the transaction. PostgreSQL supports transactional DDL fully.
func (r *Runner) rollbackMigration(mig *Migration, state *SchemaState, opts RunOptions) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op if already committed

	// Pre-apply state-only ops (SetDefaults, SetTypeMappings) from this migration so that
	// Defaults and TypeMappings are populated when generating Down SQL for other ops.
	for _, op := range mig.Operations {
		switch op.TypeName() {
		case "set_defaults", "set_type_mappings":
			if err := op.Mutate(state); err != nil {
				return fmt.Errorf("pre-applying %s state for %q: %w", op.TypeName(), mig.Name, err)
			}
		}
	}

	total := len(mig.Operations)
	for i := total - 1; i >= 0; i-- {
		op := mig.Operations[i]
		opNum := total - i
		r.provider.SetTypeMappings(state.TypeMappings)
		sqlStr, err := op.Down(r.provider, state, state.Defaults, r.migrationsDir)
		if err != nil {
			return fmt.Errorf("operation %d/%d [%s]: generating down SQL: %w", opNum, total, op.Describe(), err)
		}
		if sqlStr != "" {
			mayIgnore := canIgnoreError(op, opts) || isDropOp(op) || isCreateOp(op)
			if execErr := execWithSavepoint(tx, sqlStr, mayIgnore); execErr != nil {
				if shouldIgnoreError(op, opts, r.provider, execErr) {
					r.printf("[WARNING] op %d/%d %s — %v, skipping\n", opNum, total, op.Describe(), execErr)
				} else if isDropOp(op) && r.provider.IsAlreadyExistsError(execErr) {
					r.printf("[WARNING] op %d/%d %s — object already exists in database, skipping\n", opNum, total, op.Describe())
				} else if isCreateOp(op) && r.provider.IsNotFoundError(execErr) {
					r.printf("[WARNING] op %d/%d %s — object does not exist in database, skipping\n", opNum, total, op.Describe())
				} else {
					return fmt.Errorf("operation %d/%d [%s]: %w\n  SQL: %s", opNum, total, op.Describe(), execErr, sqlStr)
				}
			}
		}
	}

	if err := r.recorder.RecordRolledBackTx(tx, mig.Name); err != nil {
		return err
	}

	return tx.Commit()
}

// canIgnoreError returns true when the operation MIGHT have its error ignored,
// without inspecting the actual error. Used to decide whether to wrap the SQL
// execution in a SAVEPOINT (required for PostgreSQL, which aborts the entire
// transaction after any error).
func canIgnoreError(op Operation, opts RunOptions) bool {
	if ei, ok := op.(ErrorIgnorer); ok && ei.ShouldIgnoreErrors() {
		return true
	}
	return opts.WarnOnMissingDrop && isDropOp(op)
}

// shouldIgnoreError returns true when a SQL execution error should be logged as
// a warning rather than aborting the migration. This happens when:
//   - The operation has IgnoreErrors set (unconditional), or
//   - WarnOnMissingDrop is enabled AND the operation is a drop AND the error
//     indicates the object doesn't exist in the database.
func shouldIgnoreError(op Operation, opts RunOptions, p providers.Provider, execErr error) bool {
	if ei, ok := op.(ErrorIgnorer); ok && ei.ShouldIgnoreErrors() {
		return true
	}
	return opts.WarnOnMissingDrop && isDropOp(op) && p.IsNotFoundError(execErr)
}

// execWithSavepoint executes SQL within a SAVEPOINT when mayFail is true,
// so that a failed statement does not poison the surrounding transaction
// (required for PostgreSQL). When mayFail is false it executes directly.
func execWithSavepoint(tx *sql.Tx, sqlStr string, mayFail bool) error {
	if !mayFail {
		_, err := tx.Exec(sqlStr)
		return err
	}
	if _, err := tx.Exec("SAVEPOINT ignore_errors"); err != nil {
		_, execErr := tx.Exec(sqlStr)
		return execErr
	}
	if _, err := tx.Exec(sqlStr); err != nil {
		_, _ = tx.Exec("ROLLBACK TO SAVEPOINT ignore_errors")
		return err
	}
	_, _ = tx.Exec("RELEASE SAVEPOINT ignore_errors")
	return nil
}

// isDropOp returns true for operations that remove a database object.
// Only drop operations trigger WarnOnMissingDrop — all other failures stop immediately.
func isDropOp(op Operation) bool {
	switch op.TypeName() {
	case "drop_table", "drop_field", "drop_index", "drop_foreign_key":
		return true
	default:
		return false
	}
}

// isCreateOp returns true for operations that create a database object in
// their Up direction (create_table, add_field, add_index, add_foreign_key).
// During rollback, these operations' Down SQL drops objects, so a "not found"
// error can be safely skipped.
func isCreateOp(op Operation) bool {
	switch op.TypeName() {
	case "create_table", "add_field", "add_index", "add_foreign_key":
		return true
	default:
		return false
	}
}
