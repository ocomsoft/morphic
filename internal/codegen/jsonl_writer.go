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

package codegen

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// WriteTableJSONL writes rows as JSONL (one JSON object per line) to the given
// path. Keys in each row are sorted alphabetically for stable git diffs.
func WriteTableJSONL(path string, rows []map[string]any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating JSONL file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for i, row := range rows {
		sorted := sortedJSONRow(row)
		data, err := json.Marshal(sorted)
		if err != nil {
			return fmt.Errorf("row %d: marshaling JSON: %w", i, err)
		}
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("row %d: writing: %w", i, err)
		}
		if _, err := f.WriteString("\n"); err != nil {
			return fmt.Errorf("row %d: writing newline: %w", i, err)
		}
	}
	return nil
}

// sortedJSONRow returns an ordered map representation that json.Marshal
// will serialize with keys in alphabetical order.
func sortedJSONRow(row map[string]any) json.Marshaler {
	return &orderedMap{data: row}
}

// orderedMap is a map[string]any that marshals JSON keys in sorted order.
type orderedMap struct {
	data map[string]any
}

// MarshalJSON serializes the map with keys in alphabetical order.
func (m *orderedMap) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := []byte{'{'}
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		valJSON, err := json.Marshal(m.data[k])
		if err != nil {
			return nil, err
		}
		buf = append(buf, keyJSON...)
		buf = append(buf, ':')
		buf = append(buf, valJSON...)
	}
	buf = append(buf, '}')
	return buf, nil
}
