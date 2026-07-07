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

package migrate_test

import (
	"database/sql"
	"io"
	"os"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ocomsoft/morphic/internal/providers/sqlite"
	"github.com/ocomsoft/morphic/migrate"
)

// openTestDB opens an in-memory SQLite database for testing.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("opening SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// buildTestRunner creates a Runner with the given registry against an in-memory SQLite db.
// Returns the runner, the recorder (for verification), and the db.
func buildTestRunner(t *testing.T, reg *migrate.Registry) (*migrate.Runner, *migrate.MigrationRecorder, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	p := sqlite.New()
	runner := migrate.NewRunner(g, p, db, recorder, io.Discard, "")
	return runner, recorder, db
}

// suppressStdout redirects stdout to /dev/null for the duration of the test.
// Returns a cleanup function that restores the original stdout.
func suppressStdout(t *testing.T) func() {
	t.Helper()
	origStdout := os.Stdout
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("failed to open /dev/null: %v", err)
	}
	os.Stdout = devNull
	return func() {
		os.Stdout = origStdout
		_ = devNull.Close()
	}
}

// --- Recorder tests ---

func TestRecorder_EnsureTable(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	// Calling EnsureTable again should not error (IF NOT EXISTS)
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable (second call): %v", err)
	}
}

func TestRecorder_GetApplied_Empty(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected empty applied set, got %d entries", len(applied))
	}
}

func TestRecorder_RecordApplied(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if err := recorder.RecordApplied("0001_initial"); err != nil {
		t.Fatalf("RecordApplied: %v", err)
	}
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] {
		t.Fatal("expected 0001_initial to be recorded")
	}
}

func TestRecorder_RecordApplied_Duplicate(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if err := recorder.RecordApplied("0001_initial"); err != nil {
		t.Fatalf("RecordApplied: %v", err)
	}
	// Duplicate insert should fail due to UNIQUE constraint
	err := recorder.RecordApplied("0001_initial")
	if err == nil {
		t.Fatal("expected error for duplicate insert")
	}
}

func TestRecorder_RecordRolledBack(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if err := recorder.RecordApplied("0001_initial"); err != nil {
		t.Fatalf("RecordApplied: %v", err)
	}
	if err := recorder.RecordRolledBack("0001_initial"); err != nil {
		t.Fatalf("RecordRolledBack: %v", err)
	}
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if applied["0001_initial"] {
		t.Fatal("expected 0001_initial to be removed after rollback")
	}
}

func TestRecorder_Fake(t *testing.T) {
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	if err := recorder.Fake("0001_initial"); err != nil {
		t.Fatalf("Fake: %v", err)
	}
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] {
		t.Fatal("expected 0001_initial in history after Fake")
	}
}

// --- Runner Up tests ---

func TestRunner_Up_SingleMigration(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	runner, recorder, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify table exists by inserting data
	if _, err := db.Exec("INSERT INTO users (email) VALUES ('test@example.com')"); err != nil {
		t.Fatalf("expected users table to exist after Up: %v", err)
	}

	// Verify recorded
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] {
		t.Fatal("expected 0001_initial to be recorded as applied")
	}
}

func TestRunner_Up_MultipleMigrations(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_add_phone",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.AddField{
				Table: "users",
				Field: migrate.Field{Name: "phone", Type: "varchar", Length: 20, Nullable: true},
			},
		},
	})

	runner, recorder, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify both columns exist
	if _, err := db.Exec("INSERT INTO users (email, phone) VALUES ('a@b.com', '1234')"); err != nil {
		t.Fatalf("expected both columns to exist: %v", err)
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] || !applied["0002_add_phone"] {
		t.Fatal("expected both migrations to be recorded")
	}
}

func TestRunner_Up_ToTarget(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_second",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "posts",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, recorder, _ := buildTestRunner(t, reg)

	// Apply only up to 0001_initial
	if err := runner.Up("0001_initial", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up to target: %v", err)
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] {
		t.Fatal("expected 0001_initial to be applied")
	}
	if applied["0002_second"] {
		t.Fatal("expected 0002_second to NOT be applied")
	}
}

func TestRunner_Up_Idempotent(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, _, _ := buildTestRunner(t, reg)

	// Apply twice -- second call should be a no-op
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("second Up (idempotent): %v", err)
	}
}

// --- Runner Down tests ---

func TestRunner_Down_SingleStep(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	runner, recorder, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if err := runner.Down(1, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down: %v", err)
	}

	// Verify table is gone
	if _, err := db.Exec("SELECT 1 FROM users"); err == nil {
		t.Fatal("expected users table to be dropped after Down")
	}

	// Verify unrecorded
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if applied["0001_initial"] {
		t.Fatal("expected 0001_initial to be removed from history after Down")
	}
}

func TestRunner_Down_MultipleSteps(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_posts",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "posts",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, recorder, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Roll back both
	if err := runner.Down(2, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down: %v", err)
	}

	// Both tables should be gone
	if _, err := db.Exec("SELECT 1 FROM posts"); err == nil {
		t.Fatal("expected posts table to be dropped")
	}
	if _, err := db.Exec("SELECT 1 FROM users"); err == nil {
		t.Fatal("expected users table to be dropped")
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected no applied migrations, got %d", len(applied))
	}
}

func TestRunner_Down_ToTarget(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_posts",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "posts",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, recorder, _ := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Roll back to 0001_initial (exclusive: stops before rolling back 0001_initial)
	if err := runner.Down(0, "0001_initial", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down to target: %v", err)
	}

	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	// 0002_posts comes first in reverse, and "to" is 0001_initial
	// Down stops when mig.Name == to, so 0001_initial stays applied
	if !applied["0001_initial"] {
		t.Fatal("expected 0001_initial to remain applied")
	}
}

// --- Runner Status tests ---

func TestRunner_Status(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, _, _ := buildTestRunner(t, reg)

	// Status should work even with no applied migrations
	if err := runner.Status(); err != nil {
		t.Fatalf("Status: %v", err)
	}
}

// --- Runner ShowSQL tests ---

func TestRunner_ShowSQL(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, _, _ := buildTestRunner(t, reg)

	if err := runner.ShowSQL(); err != nil {
		t.Fatalf("ShowSQL: %v", err)
	}
}

func TestRunner_ShowSQL_SkipsApplied(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_posts",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "posts",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
				},
			},
		},
	})

	runner, _, _ := buildTestRunner(t, reg)

	// Apply first migration
	if err := runner.Up("0001_initial", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// ShowSQL should only show pending (0002_posts)
	if err := runner.ShowSQL(); err != nil {
		t.Fatalf("ShowSQL: %v", err)
	}
}

// --- Runner with RunSQL operation ---

func TestRunner_Up_RunSQL(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.RunSQL{
				ForwardSQL:  "CREATE TABLE raw_table (id INTEGER PRIMARY KEY, name TEXT);",
				BackwardSQL: "DROP TABLE raw_table;",
			},
		},
	})

	runner, _, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up with RunSQL: %v", err)
	}

	// Verify table was created by raw SQL
	if _, err := db.Exec("INSERT INTO raw_table (name) VALUES ('test')"); err != nil {
		t.Fatalf("expected raw_table to exist: %v", err)
	}
}

func TestRunner_Down_RunSQL(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.RunSQL{
				ForwardSQL:  "CREATE TABLE raw_table (id INTEGER PRIMARY KEY, name TEXT);",
				BackwardSQL: "DROP TABLE raw_table;",
			},
		},
	})

	runner, _, db := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := runner.Down(1, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down with RunSQL: %v", err)
	}

	// Verify table was dropped
	if _, err := db.Exec("SELECT 1 FROM raw_table"); err == nil {
		t.Fatal("expected raw_table to be dropped")
	}
}

// --- Provider bridge tests ---

func TestBuildDSN_PostgreSQL(t *testing.T) {
	// buildDSN and driverName are not exported, but we can test them
	// indirectly through App. These tests verify the App integration path.
	cfg := migrate.Config{
		DatabaseType: "postgresql",
		DBHost:       "localhost",
		DBPort:       "5432",
		DBUser:       "testuser",
		DBPassword:   "testpass",
		DBName:       "testdb",
		DBSSLMode:    "disable",
	}
	app := migrate.NewAppWithRegistry(cfg, migrate.NewRegistry())
	// We can't connect to a real PostgreSQL in unit tests,
	// but we verify the app was created successfully
	if app == nil {
		t.Fatal("expected non-nil App for PostgreSQL config")
	}
}

func TestBuildDSN_MySQL(t *testing.T) {
	cfg := migrate.Config{
		DatabaseType: "mysql",
		DBHost:       "localhost",
		DBPort:       "3306",
		DBUser:       "testuser",
		DBPassword:   "testpass",
		DBName:       "testdb",
	}
	app := migrate.NewAppWithRegistry(cfg, migrate.NewRegistry())
	if app == nil {
		t.Fatal("expected non-nil App for MySQL config")
	}
}

func TestRunner_Up_WarnOnMissingDrop_SkipsIfNotFound(t *testing.T) {
	db := openTestDB(t)
	p := sqlite.New()
	rec := migrate.NewMigrationRecorder(db, p)
	if err := rec.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_drop_users",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.DropTable{Name: "users"}, // table does not exist in DB
		},
	})

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	runner := migrate.NewRunner(g, p, db, rec, io.Discard, "")

	// Without flag — should fail because table doesn't exist
	err = runner.Up("", migrate.RunOptions{})
	if err == nil {
		t.Fatal("expected error when table does not exist and WarnOnMissingDrop is false")
	}

	// Reset history so we can attempt again
	if _, err := db.Exec("DELETE FROM morphic_history"); err != nil {
		t.Fatalf("resetting history: %v", err)
	}

	// With flag — should warn and succeed
	restore := suppressStdout(t)
	defer restore()
	err = runner.Up("", migrate.RunOptions{WarnOnMissingDrop: true})
	if err != nil {
		t.Fatalf("expected no error with WarnOnMissingDrop=true, got: %v", err)
	}
}

// TestRunner_Down_ResolvesDefaultsViaSetDefaults is a regression test for the bug where
// rollbackMigration received a pre-migration state (without SetDefaults applied) so
// DropTable.Down emitted a literal symbolic default (e.g. "now") instead of the SQL
// expression ("CURRENT_TIMESTAMP"). The fix pre-applies SetDefaults/SetTypeMappings ops
// from the migration before running the reverse loop.
func TestRunner_Down_ResolvesDefaultsViaSetDefaults(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	// 0001: just creates a base table (no defaults)
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name:   "base",
				Fields: []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}},
			},
		},
	})
	// 0002: SetDefaults + CreateTable with symbolic default
	reg.Register(&migrate.Migration{
		Name:         "0002_add_logs",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.SetDefaults{Defaults: map[string]string{"now": "CURRENT_TIMESTAMP"}},
			&migrate.CreateTable{
				Name: "logs",
				Fields: []migrate.Field{
					{Name: "id", Type: "integer", PrimaryKey: true},
					// Default is a symbolic key that must be resolved via SetDefaults
					{Name: "created_at", Type: "text", Default: "now", Nullable: true},
				},
			},
		},
	})

	runner, _, _ := buildTestRunner(t, reg)

	// Apply both migrations
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Roll back 0002 — this must resolve "now" → "CURRENT_TIMESTAMP" in DropTable.Down
	if err := runner.Down(1, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down: %v — rollbackMigration did not resolve symbolic defaults", err)
	}
}

// TestRunner_Up_TransactionRollback verifies that when a migration fails
// mid-way, neither the partial SQL changes nor the history record are committed.
func TestRunner_Up_TransactionRollback(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			// First op succeeds: creates a table.
			&migrate.RunSQL{
				ForwardSQL:  "CREATE TABLE txn_test (id INTEGER PRIMARY KEY);",
				BackwardSQL: "DROP TABLE txn_test;",
			},
			// Second op fails: invalid SQL.
			&migrate.RunSQL{
				ForwardSQL:  "THIS IS NOT VALID SQL;",
				BackwardSQL: "",
			},
		},
	})

	runner, recorder, _ := buildTestRunner(t, reg)

	err := runner.Up("", migrate.RunOptions{})
	if err == nil {
		t.Fatal("expected Up to return an error")
	}

	// Migration must NOT be recorded as applied.
	applied, recErr := recorder.GetApplied()
	if recErr != nil {
		t.Fatalf("GetApplied: %v", recErr)
	}
	if applied["0001_initial"] {
		t.Error("migration was recorded as applied despite failure — transaction not rolled back")
	}
}

// TestRunner_Down_TransactionRollback verifies that when a rollback fails
// mid-way, the history record is not removed.
func TestRunner_Down_TransactionRollback(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.RunSQL{
				ForwardSQL:  "CREATE TABLE txn_down_test (id INTEGER PRIMARY KEY);",
				BackwardSQL: "THIS IS NOT VALID SQL;", // down SQL fails
			},
		},
	})

	runner, recorder, _ := buildTestRunner(t, reg)

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	err := runner.Down(1, "", migrate.RunOptions{})
	if err == nil {
		t.Fatal("expected Down to return an error")
	}

	// Migration must still be recorded as applied since rollback failed.
	applied, recErr := recorder.GetApplied()
	if recErr != nil {
		t.Fatalf("GetApplied: %v", recErr)
	}
	if !applied["0001_initial"] {
		t.Error("migration was removed from history despite rollback failure — transaction not rolled back")
	}
}

func TestNewRunner_NilSafe(t *testing.T) {
	// Verify that NewRunner doesn't panic with valid arguments
	db := openTestDB(t)
	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	reg := migrate.NewRegistry()
	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	p := sqlite.New()
	runner := migrate.NewRunner(g, p, db, recorder, nil, "")
	if runner == nil {
		t.Fatal("expected non-nil Runner")
	}
}

func TestUp_FakeInitial_AllTablesExist(t *testing.T) {
	db := openTestDB(t)

	// Pre-create tables that the migration would create
	_, err := db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:    "0001_initial",
		Initial: true,
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatal(err)
	}

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, &buf, "")
	if err := runner.Up("", migrate.RunOptions{FakeInitial: true}); err != nil {
		t.Fatalf("Up with FakeInitial: %v", err)
	}

	if !strings.Contains(buf.String(), "faked (tables already exist)") {
		t.Errorf("expected 'faked' message, got: %s", buf.String())
	}

	applied, _ := recorder.GetApplied()
	if !applied["0001_initial"] {
		t.Error("expected 0001_initial to be recorded as applied")
	}
}

func TestUp_FakeInitial_TableMissing(t *testing.T) {
	db := openTestDB(t)

	// Do NOT pre-create the table — migration should apply normally
	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:    "0001_initial",
		Initial: true,
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatal(err)
	}

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, &buf, "")
	if err := runner.Up("", migrate.RunOptions{FakeInitial: true}); err != nil {
		t.Fatalf("Up with FakeInitial: %v", err)
	}

	if !strings.Contains(buf.String(), "done") {
		t.Errorf("expected 'done' message (normal apply), got: %s", buf.String())
	}
}

func TestUp_FakeInitial_ColumnMissing(t *testing.T) {
	db := openTestDB(t)

	// Create table with only id column, missing email
	_, err := db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY)")
	if err != nil {
		t.Fatal(err)
	}

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:    "0001_initial",
		Initial: true,
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatal(err)
	}

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, &buf, "")
	// Column missing → should apply normally, which will fail since table exists
	// but with different schema. The important thing is it does NOT fake it.
	_ = runner.Up("", migrate.RunOptions{FakeInitial: true})

	// We expect either an error (table already exists) or normal apply — NOT a fake
	output := buf.String()
	if strings.Contains(output, "faked") {
		t.Error("should NOT fake when expected columns are missing")
	}
}

func TestUp_FakeInitial_NonInitialMigration(t *testing.T) {
	db := openTestDB(t)

	// Pre-create the table
	_, err := db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:    "0001_add_users",
		Initial: false, // NOT initial
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatal(err)
	}

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, &buf, "")
	// Non-initial migration should NOT be faked even with --fake-initial
	_ = runner.Up("", migrate.RunOptions{FakeInitial: true})

	output := buf.String()
	if strings.Contains(output, "faked") {
		t.Error("should NOT fake non-initial migrations")
	}
}

func TestUp_FakeInitial_ExtraColumnsOK(t *testing.T) {
	db := openTestDB(t)

	// Table has extra columns not in migration — should still fake
	_, err := db.Exec("CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT, extra_col TEXT)")
	if err != nil {
		t.Fatal(err)
	}

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:    "0001_initial",
		Initial: true,
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name: "users",
				Fields: []migrate.Field{
					{Name: "id", Type: "uuid", PrimaryKey: true},
					{Name: "email", Type: "varchar", Length: 255},
				},
			},
		},
	})

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatal(err)
	}

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, &buf, "")
	if err := runner.Up("", migrate.RunOptions{FakeInitial: true}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if !strings.Contains(buf.String(), "faked") {
		t.Errorf("expected fake with extra columns, got: %s", buf.String())
	}
}
