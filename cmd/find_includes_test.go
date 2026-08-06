package cmd

import "testing"

func TestIsSchemaFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"schema.star", true},
		{"schema.yaml", true},
		{"schema.json", false},
		{"schema.sql", false},
		{"other.star", false},
		{"schema.star.bak", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSchemaFile(tt.name)
			if got != tt.want {
				t.Errorf("isSchemaFile(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSchemaTypeLabel(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"schema.star", "starlark"},
		{"schema.yaml", "yaml"},
		{"other.txt", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schemaTypeLabel(tt.name)
			if got != tt.want {
				t.Errorf("schemaTypeLabel(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
