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
package struct2schema

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ocomsoft/morphic/internal/types"
	"github.com/ocomsoft/morphic/internal/version"
)

// ProcessorConfig holds configuration for the struct2schema processor
type ProcessorConfig struct {
	InputDir   string
	OutputFile string
	ConfigFile string
	TargetDB   string
	DryRun     bool
	Verbose    bool
}

// Processor handles the conversion from Go structs to YAML schema
type Processor struct {
	config        ProcessorConfig
	scanner       *Scanner
	typeMapper    *TypeMapper
	tagParser     *TagParser
	generator     *Generator
	writer        *Writer
	relationships *RelationshipDetector
}

// NewProcessor creates a new struct2schema processor
func NewProcessor(config ProcessorConfig) (*Processor, error) {
	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	processor := &Processor{
		config: config,
	}

	// Initialize components
	var err error
	processor.scanner = NewScanner(config.Verbose)
	processor.typeMapper, err = NewTypeMapper(config.ConfigFile, config.TargetDB)
	if err != nil {
		return nil, fmt.Errorf("failed to create type mapper: %w", err)
	}
	processor.tagParser = NewTagParser()
	processor.generator = NewGenerator(processor.typeMapper, processor.tagParser, config.Verbose)
	processor.writer = NewWriter(config.Verbose)
	processor.relationships = NewRelationshipDetector(config.Verbose)

	return processor, nil
}

// Process executes the complete struct2schema conversion pipeline
func (p *Processor) Process() error {
	if p.config.Verbose {
		fmt.Printf("Scanning directory: %s\n", p.config.InputDir)
	}

	// Step 1: Scan for Go files and parse structs
	structs, err := p.scanner.ScanDirectory(p.config.InputDir)
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	if len(structs) == 0 {
		fmt.Println("No Go structs found in the specified directory")
		return nil
	}

	if p.config.Verbose {
		fmt.Printf("Found %d struct(s) across Go files\n", len(structs))
		for _, s := range structs {
			fmt.Printf("  - %s\n", s.Name)
		}
	}

	// Step 2: Detect relationships between structs
	if p.config.Verbose {
		fmt.Println("Analyzing struct relationships...")
	}
	relationships := p.relationships.DetectRelationships(structs)

	// Step 3: Generate schema from structs
	if p.config.Verbose {
		fmt.Println("Generating Starlark schema...")
	}
	schema, err := p.generator.GenerateSchema(structs, relationships)
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	// Add version information
	schema.Database.MigrationVersion = version.GetVersion()

	// Step 4: Output the schema
	if p.config.DryRun {
		fmt.Println("=== Generated Schema (Dry Run) ===")
		return p.writer.PreviewSchema(schema)
	}

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(p.config.OutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write the schema file
	return p.writer.WriteSchema(schema, p.config.OutputFile)
}

func validateConfig(config ProcessorConfig) error {
	// Check if input directory exists
	if _, err := os.Stat(config.InputDir); os.IsNotExist(err) {
		return fmt.Errorf("input directory does not exist: %s", config.InputDir)
	}

	// Check if target database is valid
	if !types.IsValidDatabase(config.TargetDB) {
		return fmt.Errorf("invalid target database: %s", config.TargetDB)
	}

	// Check config file exists if provided
	if config.ConfigFile != "" {
		if _, err := os.Stat(config.ConfigFile); os.IsNotExist(err) {
			return fmt.Errorf("config file does not exist: %s", config.ConfigFile)
		}
	}

	return nil
}
