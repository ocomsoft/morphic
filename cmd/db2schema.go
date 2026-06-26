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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ocomsoft/morphic/internal/codegen"
	"github.com/ocomsoft/morphic/internal/config"
	"github.com/ocomsoft/morphic/internal/providers"
	"github.com/ocomsoft/morphic/internal/types"
	yamlpkg "github.com/ocomsoft/morphic/internal/yaml"
)

var (
	// Database connection flags
	host     string
	port     int
	database string
	username string
	password string
	sslmode  string

	// Output flags
	output string
)

// db2schemaCmd represents the db2schema command
var db2schemaCmd = &cobra.Command{
	Use:     "db-to-schema",
	Aliases: []string{"db2schema"},
	GroupID: "convert",
	Short:   "Extract database schema to Starlark schema file",
	Long: `Extract database schema information from a PostgreSQL database and generate
a Starlark schema file compatible with morphic.

This command connects to a PostgreSQL database, reads the INFORMATION_SCHEMA
tables, and extracts complete metadata including:

- All user tables in the public schema
- Field information (name, data type, length, precision, scale, nullable)
- Primary key constraints
- Foreign key relationships with ON DELETE actions
- Indexes (including unique indexes)
- Default values (converted to morphic format)

Database Connection:
The command supports two ways to specify database connection:
1. Use individual flags (--host, --port, --database, --username, --password, --sslmode)
2. Use existing config file settings (default: migrations/morphic.config.yaml)

Command-line flags take precedence over config file settings.

Output:
By default, the schema is written to 'schema.star' in the current directory.
Use the --output flag to specify a different file path.

Examples:
  # Extract schema using individual connection flags
  morphic db2schema --host=localhost --port=5432 --database=myapp --username=user --password=pass

  # Extract schema using config file settings
  morphic db2schema --config=migrations/morphic.config.yaml

  # Extract schema to specific output file
  morphic db2schema --output=extracted_schema.star --host=localhost --database=myapp

  # Extract with verbose output
  morphic db2schema --verbose --host=localhost --database=myapp

Supported Databases:
- PostgreSQL (full support)
- Other databases: placeholder implementations (will be added in future versions)

The generated Starlark file follows the morphic schema format and can be used
directly with other morphic commands.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDB2Schema(cmd, args)
	},
}

// runDB2Schema executes the db2schema command
func runDB2Schema(cmd *cobra.Command, args []string) error {
	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Extracting database schema to Starlark\n")
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "======================================\n")
	}

	// Parse database type from config or default to PostgreSQL
	var dbType types.DatabaseType
	var connectionString string
	var err error

	// Load config to get database type if not provided via flags
	cfg := config.LoadOrDefault(cfgFile)

	// Determine database type - use config if available, otherwise default to PostgreSQL
	if cfg.Database.Type != "" {
		dbType, err = yamlpkg.ParseDatabaseType(cfg.Database.Type)
		if err != nil {
			return fmt.Errorf("invalid database type in config: %w", err)
		}
	} else {
		// Default to PostgreSQL if no config
		dbType = types.DatabasePostgreSQL
	}

	// Build connection string from command-line flags, falling back to config default_url
	connectionString = buildConnectionString(dbType, cfg.Database.DefaultURL)

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Database type: %s\n", dbType)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Output file: %s\n", output)
	}

	// Create provider for the database type
	provider, err := providers.NewProvider(dbType, nil)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n1. Connecting to database...\n")
	}

	// Extract schema from database
	schema, err := provider.GetDatabaseSchema(connectionString)
	if err != nil {
		return fmt.Errorf("failed to extract database schema: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Successfully extracted schema with %d tables\n", len(schema.Tables))

		// Show table summary
		for _, table := range schema.Tables {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  - %s: %d fields, %d indexes\n",
				table.Name, len(table.Fields), len(table.Indexes))
		}
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n2. Converting to Starlark DSL format...\n")
	}

	// Convert schema to Starlark DSL
	dslData, err := codegen.GenerateSchemaDSL(schema)
	if err != nil {
		return fmt.Errorf("failed to generate Starlark DSL: %w", err)
	}

	if verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\n3. Writing Starlark schema file...\n")
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(output)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write Starlark file
	if err := os.WriteFile(output, []byte(dslData), 0644); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Database schema successfully extracted to: %s\n", output)

	if len(schema.Tables) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nExtracted %d tables:\n", len(schema.Tables))
		for _, table := range schema.Tables {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", table.Name)
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nYou can now use this schema file with other morphic commands.\n")

	return nil
}

// buildConnectionString builds a database connection string from command-line flags.
// defaultURL is used as fallback when no flags or DATABASE_URL env var are set.
func buildConnectionString(dbType types.DatabaseType, defaultURL ...string) string {
	var fallback string
	if len(defaultURL) > 0 {
		fallback = defaultURL[0]
	}
	switch dbType {
	case types.DatabasePostgreSQL:
		return buildPostgreSQLConnectionString(fallback)
	default:
		// For unsupported databases, return empty string (will be caught later)
		return ""
	}
}

// buildPostgreSQLConnectionString builds PostgreSQL connection string from
// command flags, the DATABASE_URL environment variable, or the provided
// defaultURL fallback. If no flags are provided, DATABASE_URL is checked
// first, then defaultURL.
func buildPostgreSQLConnectionString(defaultURL string) string {
	if envURL := os.Getenv("DATABASE_URL"); envURL != "" && host == "" && database == "" && username == "" {
		return envURL
	}

	if defaultURL != "" && host == "" && database == "" && username == "" {
		return defaultURL
	}

	connHost := host
	if connHost == "" {
		connHost = "localhost"
	}

	connPort := port
	if connPort == 0 {
		connPort = 5432
	}

	connDatabase := database
	if connDatabase == "" {
		connDatabase = "postgres"
	}

	connUsername := username
	if connUsername == "" {
		connUsername = "postgres"
	}

	connSSLMode := sslmode
	if connSSLMode == "" {
		connSSLMode = "disable"
	}

	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s sslmode=%s",
		connHost, connPort, connDatabase, connUsername, connSSLMode)

	if password != "" {
		connStr += fmt.Sprintf(" password=%s", password)
	}

	return connStr
}

func init() {
	rootCmd.AddCommand(db2schemaCmd)

	// Database connection flags
	db2schemaCmd.Flags().StringVar(&host, "host", "", "Database host (default: localhost)")
	db2schemaCmd.Flags().IntVar(&port, "port", 0, "Database port (default: 5432 for PostgreSQL)")
	db2schemaCmd.Flags().StringVar(&database, "database", "", "Database name")
	db2schemaCmd.Flags().StringVar(&username, "username", "", "Database username")
	db2schemaCmd.Flags().StringVar(&password, "password", "", "Database password")
	db2schemaCmd.Flags().StringVar(&sslmode, "sslmode", "", "SSL mode (default: disable)")

	// Output flags
	db2schemaCmd.Flags().StringVar(&output, "output", "schema.star", "Output Starlark schema file path")

	// Common flags
	db2schemaCmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed processing information")
}
