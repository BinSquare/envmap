package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
