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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/interp"
	"github.com/ocomsoft/morphic/migrate"
)

// migrateCmd interprets the migrations module with yaegi and runs the embedded
// migrate.App in-process. DisableFlagParsing passes every argument straight to
// the App, so each of its subcommands works unchanged.
var migrateCmd = &cobra.Command{
	Use:     "migrate [args...]",
	GroupID: "runtime",
	Short:   "Run migrations in-process via the yaegi interpreter",
	Long: `Load the migrations directory with the yaegi interpreter and run the embedded
migrate App with the provided arguments. No Go toolchain is invoked — the
migration .go files are interpreted in-process. All subcommands the App
supports are available:

  morphic migrate up
  morphic migrate up --to 0005_add_index
  morphic migrate down --steps 2
  morphic migrate status
  morphic migrate showsql
  morphic migrate fake 0001_initial
  morphic migrate dag`,
	DisableFlagParsing: true,
	SilenceErrors:      true,
	RunE: func(_ *cobra.Command, args []string) error {
		cfg := config.LoadOrDefault(configFile)
		return ExecuteMigrate(cfg.Migration.Directory, cfg.Database.DefaultURL, args)
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}

// ExecuteMigrate loads migrationsDir with the yaegi interpreter and runs the
// embedded migrate.App with the provided args. defaultURL is used as the
// fallback database URL when the DATABASE_URL env var is not set.
func ExecuteMigrate(migrationsDir string, defaultURL string, args []string) error {
	reg, err := interp.LoadRegistry(migrationsDir)
	if err != nil {
		return fmt.Errorf("loading migrations: %w", err)
	}
	app := migrate.NewAppWithRegistry(migrate.Config{
		DatabaseType:  migrate.EnvOr("DB_TYPE", "postgresql"),
		DatabaseURL:   migrate.EnvOr("DATABASE_URL", defaultURL),
		MigrationsDir: migrationsDir,
	}, reg)
	return app.Run(args)
}
