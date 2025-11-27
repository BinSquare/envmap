package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func SpawnWithEnv(ctx context.Context, command string, args []string, secretEnv map[string]string) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	merged := os.Environ()
	for k, v := range secretEnv {
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
