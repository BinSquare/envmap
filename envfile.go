package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// parseDotEnv parses a .env file and returns a map of key-value pairs.
// It handles:
//   - Comments (lines starting with #)
//   - Inline comments (KEY=value # comment)
//   - Quoted values (single and double quotes)
//   - Export prefix (export KEY=value)
//   - Whitespace around = sign
func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	out := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle 'export' prefix
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimPrefix(line, "export ")
			line = strings.TrimSpace(line)
		}

		// Find the = separator
		eqIndex := strings.Index(line, "=")
		if eqIndex == -1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIndex])
		val := strings.TrimSpace(line[eqIndex+1:])

		// Skip invalid keys
		if key == "" {
			continue
		}

		// Parse the value, handling quotes and inline comments
		val = parseEnvValue(val)

		out[key] = val
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// parseEnvValue parses a value from a .env file, handling quotes and inline comments.
func parseEnvValue(val string) string {
	if val == "" {
		return ""
	}

	// Handle double-quoted values
	if strings.HasPrefix(val, `"`) {
		// Find the closing quote, accounting for escaped quotes
		endQuote := findClosingQuote(val[1:], '"')
		if endQuote != -1 {
			// Extract value between quotes, handling escape sequences
			val = val[1 : endQuote+1]
			val = strings.ReplaceAll(val, `\"`, `"`)
			val = strings.ReplaceAll(val, `\\`, `\`)
			val = strings.ReplaceAll(val, `\n`, "\n")
			val = strings.ReplaceAll(val, `\t`, "\t")
			return val
		}
		// No closing quote found, remove opening quote
		val = val[1:]
	}

	// Handle single-quoted values (no escape processing)
	if strings.HasPrefix(val, `'`) {
		endQuote := strings.Index(val[1:], `'`)
		if endQuote != -1 {
			return val[1 : endQuote+1]
		}
		// No closing quote found, remove opening quote
		val = val[1:]
	}

	// Handle inline comments (only for unquoted values)
	if idx := strings.Index(val, " #"); idx != -1 {
		val = strings.TrimSpace(val[:idx])
	}

	return val
}

// findClosingQuote finds the position of the closing quote character,
// skipping escaped quotes (preceded by backslash).
func findClosingQuote(s string, quote rune) int {
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == quote {
			return i
		}
	}
	return -1
}
