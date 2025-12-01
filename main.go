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
	"time"

	"github.com/binsquare/envmap/provider"
	"github.com/spf13/cobra"
)

// projectConfigPath can be set via --project flag to point to a specific .envmap.yaml.
var projectConfigPath string

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
			cmd.SetContext(context.Background())
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&projectConfigPath, "project", "", "path to .envmap.yaml (auto-detects by walking up from cwd if not set)")

	cmd.AddCommand(
		newInitCmd(),
		newRunCmd(),
		newExportCmd(),
		newSetCmd(),
		newGetCmd(),
		newSyncCmd(),
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
		Use:   "run [--env ENV] -- COMMAND [ARGS...]",
		Short: "Run a command with secrets injected into the environment",
		Long: `Run a command with secrets fetched from your configured provider and injected
as environment variables. This allows running applications without .env files.

The command and its arguments must come after a -- separator.

Examples:
  envmap run -- node server.js
  envmap run --env prod -- ./my-app
  envmap run --env dev -- npm start
  envmap run -- docker compose up`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("no command specified; usage: envmap run -- COMMAND [ARGS...]")
			}
			projectCfg, _, err := loadProjectConfig()
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
			fmt.Fprintf(os.Stderr, "envmap: injecting %d secrets from env %q\n", len(secretEnv), envToUse)
			return SpawnWithEnv(cmd.Context(), args[0], args[1:], secretEnv)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to use (defaults to project default_env)")
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
			projectCfg, _, err := loadProjectConfig()
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
				keys := make([]string, 0, len(secretEnv))
				for k := range secretEnv {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("%s=%s\n", k, secretEnv[k])
				}
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(secretEnv)
			default:
				return fmt.Errorf("unknown format %q (use plain or json)", format)
			}
			return nil
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to use (defaults to project default_env)")
	c.Flags().StringVar(&format, "format", "plain", "output format: plain or json")
	return c
}

func newSetCmd() *cobra.Command {
	var envName string
	var fromFile string
	var promptSecret bool
	var deleteKey bool
	c := &cobra.Command{
		Use:   "set --env ENV KEY [--file PATH|--prompt]",
		Short: "Set a secret key/value in the configured backend without exposing value on the command line",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envName == "" {
				return errors.New("provide --env to select which environment to target")
			}
			key := args[0]
			var value string
			if deleteKey {
				value = ""
			} else {
				if fromFile != "" && promptSecret {
					return errors.New("use only one of --file or --prompt")
				}
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
			}
			projectCfg, _, err := loadProjectConfig()
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			if deleteKey {
				return DeleteSecret(cmd.Context(), projectCfg, globalCfg, envName, key)
			}
			return WriteSecret(cmd.Context(), projectCfg, globalCfg, envName, key, value)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to target")
	c.Flags().StringVar(&fromFile, "file", "", "path to file containing the secret value")
	c.Flags().BoolVar(&promptSecret, "prompt", false, "prompt for the secret (no echo)")
	c.Flags().BoolVar(&deleteKey, "delete", false, "delete the specified key from the backend")
	return c
}

func newGetCmd() *cobra.Command {
	var envName string
	var raw bool
	var all bool
	var globalAll bool
	c := &cobra.Command{
		Use:   "get --env ENV KEY",
		Short: "Get a secret from the configured backend",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalAll && !all {
				return errors.New("use --global together with --all")
			}
			projectCfg, _, err := loadProjectConfig()
			if err != nil {
				return err
			}
			globalCfg, err := LoadGlobalConfig("")
			if err != nil {
				return err
			}
			if globalAll {
				for envNameIter := range projectCfg.Envs {
					if err := printEnvSecrets(cmd.Context(), projectCfg, globalCfg, envNameIter, raw); err != nil {
						return err
					}
				}
				return nil
			}
			if envName == "" {
				return errors.New("provide --env to select which environment to target")
			}
			envToUse := envName
			if all {
				return printEnvSecrets(cmd.Context(), projectCfg, globalCfg, envToUse, raw)
			}
			if len(args) != 1 {
				return errors.New("provide KEY or use --all")
			}
			key := args[0]
			value, err := FetchSecret(cmd.Context(), projectCfg, globalCfg, envToUse, key)
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
	c.Flags().BoolVar(&all, "all", false, "print all secrets for the environment (masked by default)")
	c.Flags().BoolVar(&globalAll, "global", false, "with --all, list secrets for all environments")
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
			projectCfg, _, err := loadProjectConfig()
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
	c.Flags().BoolVar(&deleteAfter, "delete", false, "delete the source .env file after successful import")
	return c
}

func newSyncCmd() *cobra.Command {
	var envName string
	var outPath string
	var merge bool
	var keepLocal bool
	var force bool
	var backup bool
	var checkOnly bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Sync provider secrets to a .env-style file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if envName == "" {
				return errors.New("provide --env to select which environment to sync")
			}
			projectCfg, _, err := loadProjectConfig()
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
			dest := outPath
			if dest == "" {
				dest = ".env"
			}
			records, err := CollectEnvWithMetadata(cmd.Context(), projectCfg, globalCfg, envToUse)
			if err != nil {
				return err
			}
			if checkOnly {
				return checkEnvDrift(dest, records)
			}
			return syncEnvFile(dest, records, merge, keepLocal, force, backup)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment name to sync from")
	c.Flags().StringVar(&outPath, "out", ".env", "path to output .env file")
	c.Flags().BoolVar(&merge, "merge", false, "preserve keys that only exist in the existing file (provider still wins on conflicts)")
	c.Flags().BoolVar(&keepLocal, "keep-local", false, "on conflicts, keep existing file values instead of provider values (use with care)")
	c.Flags().BoolVar(&force, "force", false, "skip confirmation even if file is tracked or will be overwritten")
	c.Flags().BoolVar(&backup, "backup", true, "write a .bak file before overwriting")
	c.Flags().BoolVar(&checkOnly, "check", false, "only report drift; do not write")
	return c
}

func newKeygenCmd() *cobra.Command {
	var output string
	c := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a local-store encryption key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home dir: %w", err)
				}
				output = filepath.Join(home, ".envmap", "key")
			}
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
			projectCfg, _, err := loadProjectConfig()
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
				return fmt.Errorf("missing providers")
			}
			fmt.Println("Configuration looks good.")
			return nil
		},
	}
}

func printEnvSecrets(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName string, raw bool) error {
	records, err := CollectEnvWithMetadata(ctx, projectCfg, globalCfg, envName)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(records))
	for k := range records {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(os.Stderr, "# env: %s (%d secrets)\n", envName, len(keys))
	for _, k := range keys {
		rec := records[k]
		val := rec.Value
		if !raw {
			val = MaskValue(val)
		}
		fmt.Printf("%s=%s", k, val)
		if !rec.CreatedAt.IsZero() {
			fmt.Printf("  # created %s", rec.CreatedAt.UTC().Format(time.RFC3339))
		}
		fmt.Println()
	}
	return nil
}

func loadProjectConfig() (ProjectConfig, string, error) {
	var path string
	if projectConfigPath != "" {
		path = projectConfigPath
	} else {
		found, err := FindProjectConfig("")
		if err != nil {
			return ProjectConfig{}, "", err
		}
		path = found
	}
	cfg, err := LoadProjectConfig(path)
	if err != nil {
		return ProjectConfig{}, "", err
	}
	return cfg, path, nil
}
