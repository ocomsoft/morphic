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

// jsonl_loader.go provides a JSONL file reader for loading row data
// into UpsertData migration operations.
package migrate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// loadJSONLRows reads a JSONL file (one JSON object per line) and returns
// the rows as a slice of maps. Empty lines and comment lines (starting
// with #) are skipped.
// Numeric values are preserved with full precision: integers are returned
// as int64 and floating-point numbers as float64.
func loadJSONLRows(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loadJSONLRows: failed to open %s: %w", path, err)
	}

	defer func() {
		_ = f.Close()
	}()

	var rows []map[string]any

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		row, parseErr := parseJSONLine(line)
		if parseErr != nil {
			return nil, fmt.Errorf("loadJSONLRows: parse error on line %d of %s: %w", lineNum, path, parseErr)
		}

		rows = append(rows, row)
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("loadJSONLRows: error reading %s: %w", path, scanErr)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("loadJSONLRows: file %s contains no valid rows", path)
	}

	return rows, nil
}

// parseJSONLine decodes a single JSON line into a map, converting json.Number
// values to their appropriate Go numeric types (int64 or float64).
func parseJSONLine(line string) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(line)))
	dec.UseNumber()

	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	convertNumbers(raw)

	return raw, nil
}

// convertNumbers recursively walks a map and converts json.Number values
// to int64 (if the number has no fractional part) or float64.
func convertNumbers(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case json.Number:
			m[k] = numberToGo(val)
		case map[string]any:
			convertNumbers(val)
		case []any:
			convertSliceNumbers(val)
		}
	}
}

// convertSliceNumbers recursively converts json.Number values within a slice.
func convertSliceNumbers(s []any) {
	for i, v := range s {
		switch val := v.(type) {
		case json.Number:
			s[i] = numberToGo(val)
		case map[string]any:
			convertNumbers(val)
		case []any:
			convertSliceNumbers(val)
		}
	}
}

// numberToGo converts a json.Number to int64 if it parses as an integer,
// otherwise to float64.
func numberToGo(n json.Number) any {
	if i, err := n.Int64(); err == nil {
		return i
	}

	if f, err := n.Float64(); err == nil {
		return f
	}

	// Fallback: return the string representation.
	return n.String()
}
