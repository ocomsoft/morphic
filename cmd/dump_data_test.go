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
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// resetDumpDataFlags resets all package-level flag variables used by the
// dump-data command to their zero values. This prevents test pollution
// when rootCmd is reused across tests.
func resetDumpDataFlags() {
	dumpDataName = ""
	dumpDataDryRun = false
	dumpDataVerbose = false
	dumpDataConflictKey = nil
	dumpDataDSN = ""
	dumpDataWhere = nil
	cfgFile = ""
	// Reset shared DB connection vars (declared in db2schema.go)
	host = ""
	port = 0
	database = ""
	username = ""
	password = ""
	sslmode = ""
}

// createTestSQLiteDB creates a SQLite database at the given path, executes
// the provided setup SQL, and returns the closed database path. It fails the
// test if any SQL operation encounters an error.
func createTestSQLiteDB(t *testing.T, dbPath, setupSQL string) {
	t.Helper()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("opening SQLite DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(setupSQL); err != nil {
		t.Fatalf("executing setup SQL: %v", err)
	}
}

// writeTestConfig writes a minimal morphic config YAML to the given
// path with the specified migrations directory.
func writeTestConfig(t *testing.T, cfgPath, migrationsDir string) {
	t.Helper()

	content := "database:\n  type: sqlite\nmigration:\n  directory: " + migrationsDir + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing config file: %v", err)
	}
}

// captureStdout redirects os.Stdout to a pipe, executes the given function,
// then restores stdout and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}

	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	return buf.String()
}

// TestDumpData_DryRun verifies that dump-data with --dry-run prints the
// generated migration source to stdout without writing any file.
func TestDumpData_DryRun(t *testing.T) {
	t.Cleanup(func() {
		resetDumpDataFlags()
		rootCmd.SetArgs([]string{})
	})

	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	dbPath := filepath.Join(tmpDir, "test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Create SQLite DB with a colours table.
	createTestSQLiteDB(t, dbPath, `
		CREATE TABLE colours (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		INSERT INTO colours (id, name) VALUES (1, 'Red');
		INSERT INTO colours (id, name) VALUES (2, 'Blue');
	`)

	// Write config file.
	writeTestConfig(t, cfgPath, migrationsDir)

	// Execute dump-data in dry-run mode with stdout capture.
	// Use --conflict-key since there are no existing migrations to derive PKs from.
	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"generate", "dump-data", "colours",
		"--dsn", dbPath,
		"--conflict-key", "id",
		"--dry-run",
	})

	output := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("rootCmd.Execute: %v", err)
		}
	})

	// Verify expected content in the generated migration source.
	if !strings.Contains(output, "colours") {
		t.Errorf("expected output to contain table name 'colours', got:\n%s", output)
	}

	if !strings.Contains(output, "Red") {
		t.Errorf("expected output to contain row value 'Red', got:\n%s", output)
	}

	if !strings.Contains(output, "upsert_data") {
		t.Errorf("expected output to contain 'upsert_data', got:\n%s", output)
	}

	if !strings.Contains(output, "conflict_keys") {
		t.Errorf("expected output to contain 'conflict_keys', got:\n%s", output)
	}
}

// TestDumpData_MissingArg verifies that dump-data fails when no table name
// argument is provided (cobra.MinimumNArgs(1) rejects it).
func TestDumpData_MissingArg(t *testing.T) {
	t.Cleanup(func() {
		resetDumpDataFlags()
		rootCmd.SetArgs([]string{})
	})

	rootCmd.SetArgs([]string{"generate", "dump-data"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no table name argument provided, got nil")
	}
}

// TestResolveWhere_PerTable verifies that table:condition syntax targets only
// the named table.
func TestResolveWhere_PerTable(t *testing.T) {
	t.Cleanup(func() { dumpDataWhere = nil })

	dumpDataWhere = []string{"users:status='active'", "orders:total > 0"}
	tables := []string{"users", "orders"}

	got := resolveWhere("users", tables)
	if got != "status='active'" {
		t.Errorf("resolveWhere(users) = %q, want %q", got, "status='active'")
	}

	got = resolveWhere("orders", tables)
	if got != "total > 0" {
		t.Errorf("resolveWhere(orders) = %q, want %q", got, "total > 0")
	}

	got = resolveWhere("other", tables)
	if got != "" {
		t.Errorf("resolveWhere(other) = %q, want empty", got)
	}
}

// TestResolveWhere_Bare verifies that a condition without a table prefix
// applies to all tables.
func TestResolveWhere_Bare(t *testing.T) {
	t.Cleanup(func() { dumpDataWhere = nil })

	dumpDataWhere = []string{"active = 1"}
	tables := []string{"users", "orders"}

	got := resolveWhere("users", tables)
	if got != "active = 1" {
		t.Errorf("resolveWhere(users) = %q, want %q", got, "active = 1")
	}

	got = resolveWhere("orders", tables)
	if got != "active = 1" {
		t.Errorf("resolveWhere(orders) = %q, want %q", got, "active = 1")
	}
}

// TestResolveWhere_Combined verifies that multiple matching entries are
// joined with AND.
func TestResolveWhere_Combined(t *testing.T) {
	t.Cleanup(func() { dumpDataWhere = nil })

	dumpDataWhere = []string{"users:status='active'", "users:age > 18", "visible = 1"}
	tables := []string{"users"}

	got := resolveWhere("users", tables)
	expected := "status='active' AND age > 18 AND visible = 1"
	if got != expected {
		t.Errorf("resolveWhere(users) = %q, want %q", got, expected)
	}
}

// TestResolveWhere_Empty verifies that no --where flags returns empty string.
func TestResolveWhere_Empty(t *testing.T) {
	t.Cleanup(func() { dumpDataWhere = nil })

	dumpDataWhere = nil

	got := resolveWhere("users", []string{"users"})
	if got != "" {
		t.Errorf("resolveWhere(users) = %q, want empty", got)
	}
}

// TestResolveWhere_ColonInCondition verifies that colons inside SQL values
// (e.g. timestamps) are not misinterpreted as table:condition separators.
func TestResolveWhere_ColonInCondition(t *testing.T) {
	t.Cleanup(func() { dumpDataWhere = nil })

	dumpDataWhere = []string{"created_at > '2025-01-01 10:30:00'"}
	tables := []string{"events"}

	got := resolveWhere("events", tables)
	expected := "created_at > '2025-01-01 10:30:00'"
	if got != expected {
		t.Errorf("resolveWhere(events) = %q, want %q", got, expected)
	}
}

// TestDumpData_NoMigrationsDir_PKFromFlag verifies the fallback path where
// there are no existing migrations and primary keys come from --conflict-key.
func TestDumpData_NoMigrationsDir_PKFromFlag(t *testing.T) {
	t.Cleanup(func() {
		resetDumpDataFlags()
		rootCmd.SetArgs([]string{})
	})

	tmpDir := t.TempDir()
	// Point migrations dir to a path that does not exist.
	migrationsDir := filepath.Join(tmpDir, "nonexistent_migrations")
	dbPath := filepath.Join(tmpDir, "test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Create SQLite DB with a widgets table.
	createTestSQLiteDB(t, dbPath, `
		CREATE TABLE widgets (id INTEGER PRIMARY KEY, colour TEXT);
		INSERT INTO widgets (id, colour) VALUES (1, 'green');
	`)

	// Write config file.
	writeTestConfig(t, cfgPath, migrationsDir)

	// Execute dump-data with --conflict-key override and --dry-run.
	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"generate", "dump-data", "widgets",
		"--dsn", dbPath,
		"--conflict-key", "id",
		"--dry-run",
	})

	output := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("rootCmd.Execute: %v", err)
		}
	})

	// Verify expected content in the generated migration source.
	if !strings.Contains(output, "widgets") {
		t.Errorf("expected output to contain table name 'widgets', got:\n%s", output)
	}

	if !strings.Contains(output, "green") {
		t.Errorf("expected output to contain row value 'green', got:\n%s", output)
	}
}

// TestDumpData_WhereFlag verifies that --where filters rows in the generated
// migration output.
func TestDumpData_WhereFlag(t *testing.T) {
	t.Cleanup(func() {
		resetDumpDataFlags()
		rootCmd.SetArgs([]string{})
	})

	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	dbPath := filepath.Join(tmpDir, "test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	createTestSQLiteDB(t, dbPath, `
		CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, active INTEGER);
		INSERT INTO products (id, name, active) VALUES (1, 'Widget', 1);
		INSERT INTO products (id, name, active) VALUES (2, 'Gadget', 0);
		INSERT INTO products (id, name, active) VALUES (3, 'Doohickey', 1);
	`)

	writeTestConfig(t, cfgPath, migrationsDir)

	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"generate", "dump-data", "products",
		"--dsn", dbPath,
		"--conflict-key", "id",
		"--where", "products:active = 1",
		"--dry-run",
	})

	output := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("rootCmd.Execute: %v", err)
		}
	})

	if !strings.Contains(output, "Widget") {
		t.Errorf("expected output to contain 'Widget', got:\n%s", output)
	}
	if !strings.Contains(output, "Doohickey") {
		t.Errorf("expected output to contain 'Doohickey', got:\n%s", output)
	}
	if strings.Contains(output, "Gadget") {
		t.Errorf("expected output NOT to contain 'Gadget', got:\n%s", output)
	}
}

// TestDumpData_WhereFlagBare verifies that a bare --where (no table: prefix)
// applies the filter to all tables.
func TestDumpData_WhereFlagBare(t *testing.T) {
	t.Cleanup(func() {
		resetDumpDataFlags()
		rootCmd.SetArgs([]string{})
	})

	tmpDir := t.TempDir()
	migrationsDir := filepath.Join(tmpDir, "migrations")
	dbPath := filepath.Join(tmpDir, "test.db")
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	createTestSQLiteDB(t, dbPath, `
		CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, active INTEGER);
		INSERT INTO items (id, name, active) VALUES (1, 'A', 1);
		INSERT INTO items (id, name, active) VALUES (2, 'B', 0);
	`)

	writeTestConfig(t, cfgPath, migrationsDir)

	rootCmd.SetArgs([]string{
		"--config", cfgPath,
		"generate", "dump-data", "items",
		"--dsn", dbPath,
		"--conflict-key", "id",
		"--where", "active = 1",
		"--dry-run",
	})

	output := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err != nil {
			t.Fatalf("rootCmd.Execute: %v", err)
		}
	})

	if !strings.Contains(output, `"A"`) {
		t.Errorf("expected output to contain 'A', got:\n%s", output)
	}
	if strings.Contains(output, `"B"`) {
		t.Errorf("expected output NOT to contain 'B', got:\n%s", output)
	}
}
