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
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ocomsoft/morphic/internal/providers/sqlite"
	"github.com/ocomsoft/morphic/migrate"
)

// TestFullRoundTrip tests the complete lifecycle:
// register migrations → build graph → reconstruct state → apply → rollback → verify
func TestFullRoundTrip(t *testing.T) {
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
				Indexes: []migrate.Index{
					{Name: "idx_users_email", Fields: []string{"email"}, Unique: true},
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

	// Build graph
	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	// Reconstruct state from graph
	state, err := g.ReconstructState()
	if err != nil {
		t.Fatalf("ReconstructState: %v", err)
	}
	if len(state.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(state.Tables))
	}
	if len(state.Tables["users"].Fields) != 3 {
		t.Fatalf("expected 3 fields (id, email, phone), got %d", len(state.Tables["users"].Fields))
	}

	// DAG output
	dagOut, err := g.ToDAGOutput()
	if err != nil {
		t.Fatalf("ToDAGOutput: %v", err)
	}
	if dagOut.HasBranches {
		t.Fatal("expected no branches in linear graph")
	}
	if len(dagOut.Leaves) != 1 || dagOut.Leaves[0] != "0002_add_phone" {
		t.Fatalf("expected leaf '0002_add_phone', got %v", dagOut.Leaves)
	}

	// Run against in-memory SQLite
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sqlite3 open: %v", err)
	}
	defer func() { _ = db.Close() }()

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}

	p := sqlite.New()
	runner := migrate.NewRunner(g, p, db, recorder, io.Discard, "")

	// Apply all migrations
	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Verify schema: insert a row using both columns from both migrations
	if _, err := db.Exec("INSERT INTO users (email, phone) VALUES ('a@b.com', '0412345678')"); err != nil {
		t.Fatalf("insert failed (schema may be wrong): %v", err)
	}

	// Verify both are recorded as applied
	applied, err := recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied: %v", err)
	}
	if !applied["0001_initial"] || !applied["0002_add_phone"] {
		t.Fatalf("expected both migrations applied, got %v", applied)
	}

	// Roll back both migrations
	if err := runner.Down(2, "", migrate.RunOptions{}); err != nil {
		t.Fatalf("Down: %v", err)
	}

	// Verify table is gone
	if _, err := db.Exec("SELECT 1 FROM users"); err == nil {
		t.Fatal("expected users table to be dropped after Down(2)")
	}

	// Verify history is empty
	applied, err = recorder.GetApplied()
	if err != nil {
		t.Fatalf("GetApplied after rollback: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected empty applied set after full rollback, got %v", applied)
	}
}

// TestRoundTrip_WithMerge tests that a merge migration (empty operations, two parents)
// can be registered, graphed, and applied without error.
func TestRoundTrip_WithMerge(t *testing.T) {
	restore := suppressStdout(t)
	defer restore()

	reg := migrate.NewRegistry()
	reg.Register(&migrate.Migration{
		Name:         "0001_initial",
		Dependencies: []string{},
		Operations: []migrate.Operation{
			&migrate.CreateTable{
				Name:   "users",
				Fields: []migrate.Field{{Name: "id", Type: "integer", PrimaryKey: true}},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0002_branch_a",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.AddField{
				Table: "users",
				Field: migrate.Field{Name: "name", Type: "varchar", Length: 100, Nullable: true},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0003_branch_b",
		Dependencies: []string{"0001_initial"},
		Operations: []migrate.Operation{
			&migrate.AddField{
				Table: "users",
				Field: migrate.Field{Name: "email", Type: "varchar", Length: 255, Nullable: true},
			},
		},
	})
	reg.Register(&migrate.Migration{
		Name:         "0004_merge",
		Dependencies: []string{"0002_branch_a", "0003_branch_b"},
		Operations:   []migrate.Operation{},
	})

	g, err := migrate.BuildGraph(reg)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	if !g.HasBranches() {
		t.Fatal("expected branches before merge migration")
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sqlite3 open: %v", err)
	}
	defer func() { _ = db.Close() }()

	recorder := migrate.NewMigrationRecorder(db, sqlite.New())
	if err := recorder.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}

	runner := migrate.NewRunner(g, sqlite.New(), db, recorder, io.Discard, "")

	if err := runner.Up("", migrate.RunOptions{}); err != nil {
		t.Fatalf("Up with merge migration: %v", err)
	}

	// Verify all columns exist
	if _, err := db.Exec("INSERT INTO users (name, email) VALUES ('Alice', 'a@b.com')"); err != nil {
		t.Fatalf("insert with both columns: %v", err)
	}
}
