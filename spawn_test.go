package main

import "testing"

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "(empty)"},
		{"a", "****"},
		{"ab", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "ab****de"},
		{"secretpassword", "se****rd"},
		{"postgres://user:pass@host/db", "po****db"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := MaskValue(tt.input)
			if got != tt.expected {
				t.Errorf("MaskValue(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"path/to/file", "path/to/file"},
		{"has space", "'has space'"},
		{"has'quote", "'has'\"'\"'quote'"},
		{"special$char", "'special$char'"},
		{"multi\nline", "'multi\nline'"},
		{"", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shellQuote(tt.input)
			if got != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
