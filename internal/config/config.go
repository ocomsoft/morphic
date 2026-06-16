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

// Package config provides configuration loading and management for makemigrations.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	yaml "gopkg.in/yaml.v3"
)

// Config represents the morphic configuration
type Config struct {
	// Database configuration
	Database DatabaseConfig `yaml:"database" mapstructure:"database"`

	// Migration settings
	Migration MigrationConfig `yaml:"migration" mapstructure:"migration"`

	// Output settings
	Output OutputConfig `yaml:"output" mapstructure:"output"`
}

// DatabaseConfig contains database-related settings
type DatabaseConfig struct {
	Type       string `yaml:"type" mapstructure:"type"`               // postgresql, mysql, sqlserver, sqlite
	DefaultURL string `yaml:"default_url" mapstructure:"default_url"` // Fallback database URL when DATABASE_URL env var is not set
}

// MigrationConfig contains migration-related settings
type MigrationConfig struct {
	Directory string `yaml:"directory" mapstructure:"directory"` // Directory for migration files
	Format    string `yaml:"format" mapstructure:"format"`       // Output format: go or starlark
}

// OutputConfig contains output formatting settings
type OutputConfig struct {
	Verbose      bool `yaml:"verbose" mapstructure:"verbose"`             // Enable verbose output
	ColorEnabled bool `yaml:"color_enabled" mapstructure:"color_enabled"` // Enable colored output
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Database: DatabaseConfig{
			Type: "postgresql",
		},
		Migration: MigrationConfig{
			Directory: "migrations",
			Format:    "go",
		},
		Output: OutputConfig{
			Verbose:      false,
			ColorEnabled: true,
		},
	}
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set up environment variable binding
	v.SetEnvPrefix("MORPHIC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	cfg := DefaultConfig()
	setDefaults(v, cfg)

	// Try to read config file if it exists
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			// It's okay if config file doesn't exist, we'll use defaults
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	} else {
		// Try morphic.config first, fall back to makemigrations.config for backward compat
		v.SetConfigName("morphic.config")
		v.SetConfigType("yaml")
		v.AddConfigPath("migrations")
		v.AddConfigPath(".")

		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				// Try legacy config name
				v.SetConfigName("makemigrations.config")
				if legacyErr := v.ReadInConfig(); legacyErr != nil {
					if _, ok := legacyErr.(viper.ConfigFileNotFoundError); !ok {
						return nil, fmt.Errorf("failed to read config file: %w", legacyErr)
					}
				}
			} else {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	// Unmarshal into our config struct
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads configuration or returns default if not found
func LoadOrDefault(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		return DefaultConfig()
	}
	return cfg
}

// Save saves the configuration to a file
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := `# Morphic Configuration File
#
# This file contains configuration for the morphic tool.
# All settings can be overridden using environment variables with the prefix MORPHIC_
# For example: MORPHIC_DATABASE_TYPE=mysql
#
# For nested values, use underscores: MORPHIC_OUTPUT_COLOR_ENABLED=false
#

`

	// Write to file
	fullContent := []byte(header + string(data))
	if err := os.WriteFile(path, fullContent, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// setDefaults sets default values in viper
func setDefaults(v *viper.Viper, cfg *Config) {
	v.SetDefault("database.type", cfg.Database.Type)
	v.SetDefault("database.default_url", cfg.Database.DefaultURL)
	v.SetDefault("migration.directory", cfg.Migration.Directory)
	v.SetDefault("migration.format", cfg.Migration.Format)
	v.SetDefault("output.verbose", cfg.Output.Verbose)
	v.SetDefault("output.color_enabled", cfg.Output.ColorEnabled)
}

// GetConfigPath returns the default config file path
func GetConfigPath() string {
	return filepath.Join("migrations", "morphic.config.yaml")
}
