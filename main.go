package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/binsquare/envmap/provider"
	"github.com/spf13/cobra"
)

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "envmap",
		Short: "envMap replaces .env files with secure secret injection",
		Long:  "envMap fetches secrets from configured backends and injects them into processes without writing .env files.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Cobra executes child PersistentPreRunE; if root was invoked directly we still want cancellation contexts.
			cmd.SetContext(context.Background())
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(
		newInitCmd(),
		newRunCmd(),
		newEnvCmd(),
		newExportCmd(),
		newSetCmd(),
		newGetCmd(),
		newImportCmd(),
		newKeygenCmd(),
		newValidateCmd(),
	)

	return cmd
}

func newInitCmd() *cobra.Command {
	var globalOnly bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Interactively configure envMap",
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalOnly {
				return runInteractiveGlobalSetup(cmd.Context())
			}
			return runInteractiveInit(cmd.Context())
		},
	}
	c.Flags().BoolVar(&globalOnly, "global", false, "configure ~/.envmap/config.yaml (providers)")
	return c
}

func newRunCmd() *cobra.Command {
	var envName string
	c := &cobra.Command{
		Use:   "run -- <command>",
		Short: "Fetch secrets and run a command with injected environment",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.ArgsLenAtDash() == -1 {
				return errors.New("use envmap run -- <command> to forward args to the target process")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash == -1 || dash >= len(args) {
				return errors.New("no command provided after --")
			}
			target := args[dash:]

			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}

			envToUse, err := ResolveEnv(projectCfg, envName)
			if err != nil {
				return err
			}

			secretEnv, err := CollectEnv(cmd.Context(), projectCfg, globalCfg, envToUse)
			if err != nil {
				return err
			}

			return SpawnWithEnv(cmd.Context(), target[0], target[1:], secretEnv)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to use (defaults to project default_env)")
	return c
}

func newEnvCmd() *cobra.Command {
	var envName string
	var raw bool
	c := &cobra.Command{
		Use:   "env",
		Short: "Debug: show secrets for an environment (masked by default)",
		Long:  "Display secrets for human inspection. Values are masked by default. Use 'export' for machine-readable output.",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			envToUse, err := ResolveEnv(projectCfg, envName)
			if err != nil {
				return err
			}
			secretEnv, err := CollectEnv(cmd.Context(), projectCfg, globalCfg, envToUse)
			if err != nil {
				return err
			}

			// Sort keys for consistent output
			keys := make([]string, 0, len(secretEnv))
			for k := range secretEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			fmt.Fprintf(os.Stderr, "# env: %s (%d secrets)\n", envToUse, len(keys))
			for _, k := range keys {
				v := secretEnv[k]
				if raw {
					fmt.Printf("%s=%s\n", k, v)
				} else {
					fmt.Printf("%s=%s\n", k, MaskValue(v))
				}
			}
			return nil
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name (defaults to project default_env)")
	c.Flags().BoolVar(&raw, "raw", false, "show unmasked values (use with care)")
	return c
}

func newExportCmd() *cobra.Command {
	var envName string
	var format string
	c := &cobra.Command{
		Use:   "export",
		Short: "Export secrets to stdout for shell eval or tooling",
		Long: `Export secrets in machine-readable format to stdout.

Formats:
  plain   KEY=VAL lines, suitable for shell eval or direnv
  json    JSON object, suitable for tooling

Examples:
  eval $(envmap export --env dev)
  envmap export --env dev --format json | jq .`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			envToUse, err := ResolveEnv(projectCfg, envName)
			if err != nil {
				return err
			}
			secretEnv, err := CollectEnv(cmd.Context(), projectCfg, globalCfg, envToUse)
			if err != nil {
				return err
			}

			switch format {
			case "plain", "":
				// Sort keys for deterministic output
				keys := make([]string, 0, len(secretEnv))
				for k := range secretEnv {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				for _, k := range keys {
					// Shell-safe export format
					fmt.Printf("export %s=%s\n", k, shellQuote(secretEnv[k]))
				}
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(secretEnv); err != nil {
					return fmt.Errorf("encode json: %w", err)
				}
			default:
				return fmt.Errorf("unknown format %q; use 'plain' or 'json'", format)
			}
			return nil
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name (defaults to project default_env)")
	c.Flags().StringVar(&format, "format", "plain", "output format: plain, json")
	return c
}

// shellQuote quotes a string for safe shell use.
func shellQuote(s string) string {
	// If the string is simple, no quoting needed
	if isSimpleValue(s) {
		return s
	}
	// Use single quotes, escaping any single quotes in the value
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func isSimpleValue(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' || c == '/') {
			return false
		}
	}
	return len(s) > 0
}

func newSetCmd() *cobra.Command {
	var envName string
	var fromFile string
	var promptSecret bool
	c := &cobra.Command{
		Use:   "set --env ENV KEY [--file PATH|--prompt]",
		Short: "Set a secret key/value in the configured backend without exposing value on the command line",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envName == "" {
				return errors.New("provide --env to select which environment to target")
			}
			if fromFile != "" && promptSecret {
				return errors.New("use only one of --file or --prompt")
			}
			var value string
			if fromFile != "" {
				valueBytes, err := os.ReadFile(fromFile)
				if err != nil {
					return fmt.Errorf("read secret file: %w", err)
				}
				value = strings.TrimSpace(string(valueBytes))
			} else if promptSecret {
				v, err := readSecretFromPrompt("Secret value: ")
				if err != nil {
					return err
				}
				value = v
			} else {
				return errors.New("provide --file or --prompt to supply the secret without shell history leakage")
			}
			key := args[0]
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			return WriteSecret(cmd.Context(), projectCfg, globalCfg, envName, key, value)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to target")
	c.Flags().StringVar(&fromFile, "file", "", "path to file containing the secret value")
	c.Flags().BoolVar(&promptSecret, "prompt", false, "prompt for the secret (no echo)")
	return c
}

func newGetCmd() *cobra.Command {
	var envName string
	var raw bool
	c := &cobra.Command{
		Use:   "get --env ENV KEY",
		Short: "Get a secret from the configured backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envName == "" {
				return errors.New("provide --env to select which environment to target")
			}
			key := args[0]
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			value, err := FetchSecret(cmd.Context(), projectCfg, globalCfg, envName, key)
			if err != nil {
				return err
			}
			if raw {
				fmt.Printf("%s\n", value)
				return nil
			}
			fmt.Printf("%s\n", MaskValue(value))
			return nil
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to target")
	c.Flags().BoolVar(&raw, "raw", false, "print raw secret value (use with care)")
	return c
}

func newImportCmd() *cobra.Command {
	var envName string
	var deleteAfter bool
	c := &cobra.Command{
		Use:   "import PATH --env ENV",
		Short: "Import secrets from a .env file into a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envName == "" {
				return errors.New("provide --env to select which environment to import into")
			}
			path := args[0]
			entries, err := parseDotEnv(path)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return fmt.Errorf("no entries found in %s", path)
			}
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			fmt.Printf("Importing %d keys into env %s from %s\n", len(entries), envName, path)
			for k := range entries {
				fmt.Printf(" - %s\n", k)
			}
			for k, v := range entries {
				if err := WriteSecret(cmd.Context(), projectCfg, globalCfg, envName, k, v); err != nil {
					return err
				}
			}
			if deleteAfter {
				if err := os.Remove(path); err != nil {
					return fmt.Errorf("import succeeded but failed to delete %s: %w", path, err)
				}
				fmt.Printf("Deleted %s\n", path)
			}
			return nil
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to import into")
	c.Flags().BoolVar(&deleteAfter, "delete", false, "delete the .env file after successful import")
	return c
}

func newKeygenCmd() *cobra.Command {
	var output string
	c := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a secure encryption key for local storage",
		Long: `Generate a cryptographically secure 256-bit key for local-file provider.

The key is written to the specified file with restrictive permissions (0600).
Store this file outside your repository and back it up securely.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home dir: %w", err)
				}
				output = filepath.Join(home, ".envmap", "key")
			}

			// Check if file exists
			if _, err := os.Stat(output); err == nil {
				return fmt.Errorf("key file %s already exists; remove it first if you want to regenerate", output)
			}

			if err := provider.GenerateKeyFile(output); err != nil {
				return err
			}

			fmt.Printf("Generated encryption key: %s\n", output)
			fmt.Println("Keep this file secure and backed up. Do not commit to version control.")
			return nil
		},
	}
	c.Flags().StringVarP(&output, "output", "o", "", "output path (default: ~/.envmap/key)")
	return c
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectCfg, err := LoadProjectConfig("")
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			fmt.Printf("Project: %s\n", projectCfg.Project)
			fmt.Printf("Envs: %d (default: %s)\n", len(projectCfg.Envs), projectCfg.DefaultEnv)
			providers := globalCfg.GetProviders()
			missing := []string{}
			for envName, envCfg := range projectCfg.Envs {
				providerName := envCfg.GetProvider()
				if _, ok := providers[providerName]; !ok {
					missing = append(missing, fmt.Sprintf("%s → %s", envName, providerName))
				}
			}
			if len(missing) > 0 {
				fmt.Println("\nMissing providers in ~/.envmap/config.yaml:")
				for _, m := range missing {
					fmt.Printf("  %s\n", m)
				}
				return errors.New("configuration incomplete")
			}
			fmt.Printf("Providers: %d\n", len(providers))
			fmt.Println("✓ Configuration valid")
			return nil
		},
	}
}
