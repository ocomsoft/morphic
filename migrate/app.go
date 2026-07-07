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
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// App is the CLI application embedded in each compiled migration binary.
type App struct {
	config   Config
	root     *cobra.Command
	registry *Registry
}

// NewApp creates a new App with the given configuration.
// It uses the global registry by default.
func NewApp(cfg Config) *App {
	app := &App{config: cfg, registry: GlobalRegistry()}
	app.root = app.buildRootCommand()
	return app
}

// NewAppWithRegistry creates a new App with the given configuration and a
// specific registry. This is primarily useful for testing.
func NewAppWithRegistry(cfg Config, reg *Registry) *App {
	app := &App{config: cfg, registry: reg}
	app.root = app.buildRootCommand()
	return app
}

// Run executes the CLI with the given arguments.
func (a *App) Run(args []string) error {
	a.root.SetArgs(args)
	return a.root.Execute()
}

func (a *App) buildRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "migrate",
		Short:         "morphic migration runner",
		Long:          "Compiled migration binary -- apply, rollback, and inspect database migrations.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(a.buildDAGCommand())
	root.AddCommand(a.buildUpCommand())
	root.AddCommand(a.buildDownCommand())
	root.AddCommand(a.buildStatusCommand())
	root.AddCommand(a.buildShowSQLCommand())
	root.AddCommand(a.buildFakeCommand())

	return root
}

func (a *App) buildDAGCommand() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "dag",
		Short: "Show the migration dependency graph",
		RunE: func(_ *cobra.Command, _ []string) error {
			g, err := BuildGraph(a.registry)
			if err != nil {
				return fmt.Errorf("building graph: %w", err)
			}
			dagOut, err := g.ToDAGOutput()
			if err != nil {
				return fmt.Errorf("building DAG output: %w", err)
			}
			if outputFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(dagOut)
			}
			fmt.Print(RenderDAGASCII(dagOut))
			return nil
		},
	}
	cmd.Flags().StringVar(&outputFormat, "format", "ascii", "Output format: ascii or json")
	return cmd
}

func (a *App) buildUpCommand() *cobra.Command {
	var toMigration string
	var warnOnMissingDrop bool
	var fakeInitial bool
	cmd := &cobra.Command{
		Use:   "up",
		Short: "Apply pending migrations",
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runUp(toMigration, RunOptions{
				WarnOnMissingDrop: warnOnMissingDrop,
				FakeInitial:       fakeInitial,
			})
		},
	}
	cmd.Flags().StringVar(&toMigration, "to", "", "Apply up to this migration name")
	cmd.Flags().BoolVar(&warnOnMissingDrop, "warn-on-missing-drop", false, "Warn and continue when a drop fails because the object does not exist")
	cmd.Flags().BoolVar(&fakeInitial, "fake-initial", false, "Fake initial migrations if their tables already exist in the database")
	return cmd
}

func (a *App) buildDownCommand() *cobra.Command {
	var steps int
	var toMigration string
	var warnOnMissingDrop bool
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Rollback migrations",
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runDown(steps, toMigration, RunOptions{WarnOnMissingDrop: warnOnMissingDrop})
		},
	}
	cmd.Flags().IntVar(&steps, "steps", 1, "Number of migrations to roll back")
	cmd.Flags().StringVar(&toMigration, "to", "", "Roll back to this migration name")
	cmd.Flags().BoolVar(&warnOnMissingDrop, "warn-on-missing-drop", false, "Warn and continue when a drop fails because the object does not exist")
	return cmd
}

func (a *App) buildStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runStatus()
		},
	}
}

func (a *App) buildShowSQLCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "showsql",
		Short: "Print SQL for pending migrations without executing",
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runShowSQL()
		},
	}
}

func (a *App) buildFakeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "fake [migration-name]",
		Short: "Mark a migration as applied without running its SQL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runFake(args[0])
		},
	}
}

// --- Database helpers ---

// openDB creates a *sql.DB from the app config.
func (a *App) openDB() (*sql.DB, error) {
	dsn := a.config.DatabaseURL
	if dsn == "" {
		dsn = buildDSN(a.config)
	}
	driver := driverName(a.config.DatabaseType)
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	return db, nil
}

// buildDSN constructs a DSN from individual config fields.
func buildDSN(cfg Config) string {
	switch cfg.DatabaseType {
	case "postgresql", "postgres":
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)
	case "sqlite":
		return cfg.DBName
	default:
		return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	}
}

// driverName maps DatabaseType to SQL driver name.
func driverName(dbType string) string {
	switch dbType {
	case "mysql", "tidb":
		return "mysql"
	case "sqlserver":
		return "sqlserver"
	case "sqlite":
		return "sqlite3"
	default:
		return "postgres"
	}
}

// --- Runner wiring ---

// buildRunner creates a fully-wired Runner from the app config and registry.
// It also returns the underlying *sql.DB so the caller can close it when done.
func (a *App) buildRunner() (*Runner, *sql.DB, error) {
	reg := a.registry
	g, err := BuildGraph(reg)
	if err != nil {
		return nil, nil, fmt.Errorf("building graph: %w", err)
	}
	db, err := a.openDB()
	if err != nil {
		return nil, nil, err
	}
	p, err := BuildProviderFromType(a.config.DatabaseType)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	recorder := NewMigrationRecorder(db, p)
	if err := recorder.EnsureTable(); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return NewRunner(g, p, db, recorder, os.Stdout, a.config.MigrationsDir), db, nil
}

func (a *App) runUp(to string, opts RunOptions) error {
	r, db, err := a.buildRunner()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return r.Up(to, opts)
}

func (a *App) runDown(steps int, to string, opts RunOptions) error {
	r, db, err := a.buildRunner()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return r.Down(steps, to, opts)
}

func (a *App) runStatus() error {
	r, db, err := a.buildRunner()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return r.Status()
}

func (a *App) runShowSQL() error {
	r, db, err := a.buildRunner()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return r.ShowSQL()
}

func (a *App) runFake(name string) error {
	// Resolve partial name (e.g. "0001") to the full registered name
	// (e.g. "0001_initial") so the history entry matches what Up() looks for.
	resolved, err := a.registry.Resolve(name)
	if err != nil {
		return err
	}
	db, err := a.openDB()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	p, err := BuildProviderFromType(a.config.DatabaseType)
	if err != nil {
		return err
	}
	recorder := NewMigrationRecorder(db, p)
	if err := recorder.EnsureTable(); err != nil {
		return err
	}
	// Check if already applied so fake is idempotent.
	applied, err := recorder.GetApplied()
	if err != nil {
		return err
	}
	if applied[resolved] {
		fmt.Printf("Migration %q already marked as applied — skipping.\n", resolved)
		return nil
	}
	if err := recorder.Fake(resolved); err != nil {
		return err
	}
	fmt.Printf("Marked %q as applied (faked).\n", resolved)
	return nil
}
