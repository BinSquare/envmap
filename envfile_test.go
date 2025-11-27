package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	content := `
# Database config
DB_HOST=localhost
DB_PORT=5432
DB_PASSWORD="quoted value"
API_KEY='single quoted'

# Empty and malformed lines
EMPTY=
NO_VALUE
  WHITESPACE = spaced  
`
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("parseDotEnv: %v", err)
	}

	tests := []struct {
		key      string
		expected string
		exists   bool
	}{
		{"DB_HOST", "localhost", true},
		{"DB_PORT", "5432", true},
		{"DB_PASSWORD", "quoted value", true}, // quotes stripped
		{"API_KEY", "single quoted", true},    // single quotes stripped
		{"EMPTY", "", true},                   // empty value is valid
		{"WHITESPACE", "spaced", true},        // whitespace trimmed
		{"NO_VALUE", "", false},               // malformed, skipped
		{"COMMENT", "", false},                // comments skipped
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, ok := got[tt.key]
			if ok != tt.exists {
				t.Errorf("key %q exists = %v, want %v", tt.key, ok, tt.exists)
			}
			if ok && val != tt.expected {
				t.Errorf("got[%q] = %q, want %q", tt.key, val, tt.expected)
			}
		})
	}
}

func TestParseDotEnvEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("# only comments\n\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("parseDotEnv: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

func TestParseDotEnvNotFound(t *testing.T) {
	_, err := parseDotEnv("/nonexistent/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
