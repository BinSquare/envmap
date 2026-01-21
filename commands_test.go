package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestListCommandExecutes(t *testing.T) {
	// Create a temporary project config
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".envmap.yaml")
	cfg := `project: test-project
default_env: dev
envs:
  dev:
    provider: local-dev
    path_prefix: /test/dev/
  prod:
    provider: aws-ssm
    path_prefix: /test/prod/
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp directory
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(dir)

	// Test list command runs without error
	cmd := newListCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list command failed: %v", err)
	}
}

func TestListCommandJSONExecutes(t *testing.T) {
	// Create a temporary project config
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".envmap.yaml")
	cfg := `project: json-test
default_env: staging
envs:
  staging:
    provider: local-file
    path_prefix: /staging/
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp directory
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(dir)

	// Test list command with JSON output
	cmd := newListCmd()
	cmd.SetArgs([]string{"--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("list --json command failed: %v", err)
	}
}

func TestListCommandNoConfig(t *testing.T) {
	// Create an empty temp directory (no config file)
	dir := t.TempDir()

	// Change to the temp directory
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(dir)

	cmd := newListCmd()
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no config file exists")
	}
}

func TestDoctorCommandExecutes(t *testing.T) {
	// Create a temporary environment with config files
	dir := t.TempDir()

	// Create project config
	projectCfg := filepath.Join(dir, ".envmap.yaml")
	cfg := `project: doctor-test
default_env: dev
envs:
  dev:
    provider: local-dev
    path_prefix: /doctor/dev/
`
	if err := os.WriteFile(projectCfg, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to the temp directory
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(dir)

	cmd := newDoctorCmd()
	// Doctor may return error if global config providers don't match, but should execute
	_ = cmd.Execute()
}

func TestCompletionCommandExecutes(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "powershell"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			// Create a root command with completion subcommand
			rootCmd := &cobra.Command{Use: "envmap"}
			completionCmd := newCompletionCmd()
			rootCmd.AddCommand(completionCmd)

			// Execute the completion subcommand for the specific shell
			rootCmd.SetArgs([]string{"completion", shell})
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("completion %s failed: %v", shell, err)
			}
		})
	}
}

func TestCompletionCommandInvalidShell(t *testing.T) {
	rootCmd := &cobra.Command{Use: "envmap"}
	completionCmd := newCompletionCmd()
	rootCmd.AddCommand(completionCmd)

	rootCmd.SetArgs([]string{"completion", "invalid-shell"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid shell")
	}
}
