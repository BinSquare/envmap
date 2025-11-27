package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/binsquare/envmap/provider"
	"gopkg.in/yaml.v3"
)

func runInteractiveInit(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)

	cwd, _ := os.Getwd()
	projectDefault := filepath.Base(cwd)
	project := prompt(reader, "Project name", projectDefault)
	if project == "" {
		return fmt.Errorf("project name is required")
	}

	envName := prompt(reader, "Environment name", "dev")
	providerName := prompt(reader, "Provider name (as defined in ~/.envmap/config.yaml)", "local-store")

	pathPrefix := prompt(reader, "Path prefix (SSM) [example: /project/dev/]", fmt.Sprintf("/%s/%s/", project, envName))
	prefix := prompt(reader, "Prefix (alternative to path prefix, leave blank to use path prefix)", "")
	cfgPath := DefaultProjectConfigPath()
	overwrite := false
	if _, err := os.Stat(cfgPath); err == nil {
		resp := prompt(reader, fmt.Sprintf("%s exists. Overwrite? (y/N)", cfgPath), "N")
		overwrite = strings.ToLower(resp) == "y"
		if !overwrite {
			return fmt.Errorf("aborted; %s already exists", cfgPath)
		}
	}

	projectCfg := ProjectConfig{
		Project:    project,
		DefaultEnv: envName,
		Envs: map[string]EnvConfig{
			envName: {
				Provider:   providerName,
				PathPrefix: pathPrefix,
				Prefix:     prefix,
			},
		},
	}
	raw, err := yaml.Marshal(projectCfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("Wrote %s\n", cfgPath)

	envFile := detectEnvFile()
	useEnv := prompt(reader, fmt.Sprintf("Import secrets from detected .env file? (%s) (y/N)", envFile), "N")
	if envFile != "" && strings.ToLower(useEnv) == "y" {
		entries, err := parseDotEnv(envFile)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Printf("No keys found in %s\n", envFile)
			return nil
		}
		globalCfg, err := LoadGlobalConfig("")
		if err != nil {
			return err
		}
		for k, v := range entries {
			if err := WriteSecret(ctx, projectCfg, globalCfg, envName, k, v); err != nil {
				return err
			}
		}
		fmt.Printf("Imported %d keys from %s\n", len(entries), envFile)
	}
	return nil
}

func prompt(r *bufio.Reader, msg, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", msg, def)
	} else {
		fmt.Printf("%s: ", msg)
	}
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return def
	}
	return input
}

func detectEnvFile() string {
	candidates := []string{".env", ".env.local"}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func runInteractiveGlobalSetup(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)

	providerName := prompt(reader, "Provider name", "local-dev")
	providerType := prompt(reader, "Provider type", "local-file")
	if providerType != "local-file" {
		return fmt.Errorf("global setup currently supports provider type local-file; edit %s manually for %s providers", DefaultGlobalConfigPath(), providerType)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home dir: %w", err)
	}
	defaultStore := filepath.Join(home, ".envmap", "secrets.db")
	defaultKey := filepath.Join(home, ".envmap", "key")

	storeInput := prompt(reader, "Encrypted store path", defaultStore)
	keyInput := prompt(reader, "Key file path", defaultKey)

	storePath, err := expandPath(storeInput)
	if err != nil {
		return fmt.Errorf("resolve store path: %w", err)
	}
	keyPath, err := expandPath(keyInput)
	if err != nil {
		return fmt.Errorf("resolve key path: %w", err)
	}

	if _, err := os.Stat(keyPath); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Key %s not found; generating...\n", keyPath)
		if err := provider.GenerateKeyFile(keyPath); err != nil {
			return fmt.Errorf("generate key file: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("stat key file: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0o700); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}

	globalPath := DefaultGlobalConfigPath()
	var globalCfg GlobalConfig
	if _, err := os.Stat(globalPath); err == nil {
		cfg, err := LoadGlobalConfig(globalPath)
		if err != nil {
			return err
		}
		globalCfg = cfg
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat global config: %w", err)
	}

	if len(globalCfg.Providers) == 0 {
		globalCfg.Providers = make(map[string]provider.ProviderConfig)
		if len(globalCfg.Sources) > 0 {
			for k, v := range globalCfg.Sources {
				globalCfg.Providers[k] = v
			}
		}
	}

	globalCfg.Providers[providerName] = provider.ProviderConfig{
		Type:       providerType,
		Path:       storePath,
		Encryption: &provider.EncryptionConfig{KeyFile: keyPath},
	}

	raw, err := yaml.Marshal(globalCfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		return fmt.Errorf("create global config dir: %w", err)
	}
	if err := os.WriteFile(globalPath, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", globalPath, err)
	}

	fmt.Printf("Updated %s with provider %s (%s)\n", globalPath, providerName, providerType)
	return nil
}

func expandPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("path cannot be empty")
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	p = os.ExpandEnv(p)
	return filepath.Abs(p)
}
