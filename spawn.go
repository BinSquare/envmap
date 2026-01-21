package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func SpawnWithEnv(ctx context.Context, command string, args []string, secretEnv map[string]string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Build environment with secrets taking precedence over existing vars
	envMap := make(map[string]string)

	// First, load existing environment
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Then override with secrets (secrets take precedence)
	for k, v := range secretEnv {
		envMap[k] = v
	}

	// Convert back to slice
	merged := make([]string, 0, len(envMap))
	for k, v := range envMap {
		merged = append(merged, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = merged

	return cmd.Run()
}

func MaskValue(value string) string {
	if value == "" {
		return "(empty)"
	}
	if len(value) <= 4 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}

// shellQuote applies a minimal POSIX-safe single-quote escaping for display.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// If the string contains only safe characters, return as-is.
	for _, r := range s {
		if !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '@' || r == '+' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			// Need quoting
			goto needsQuote
		}
	}
	return s

needsQuote:
	// Escape single quotes by closing, escaping, and reopening: ' -> '\''.
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
