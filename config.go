package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/binsquare/envmap/provider"
	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	Project    string               `yaml:"project"`
	DefaultEnv string               `yaml:"default_env"`
	Envs       map[string]EnvConfig `yaml:"envs"`
}

type EnvConfig struct {
	Provider   string `yaml:"provider"`
	Source     string `yaml:"source,omitempty"` // deprecated, use Provider
	PathPrefix string `yaml:"path_prefix"`
	Prefix     string `yaml:"prefix"`
}

func (e EnvConfig) GetProvider() string {
	if e.Provider != "" {
		return e.Provider
	}
	return e.Source
}

func (e EnvConfig) ToProviderConfig() provider.EnvConfig {
	return provider.EnvConfig{
		Provider:   e.GetProvider(),
		PathPrefix: e.PathPrefix,
		Prefix:     e.Prefix,
	}
}

type GlobalConfig struct {
	Providers map[string]provider.ProviderConfig `yaml:"providers"`
	Sources   map[string]provider.ProviderConfig `yaml:"sources,omitempty"` // deprecated
}

func (g GlobalConfig) GetProviders() map[string]provider.ProviderConfig {
	if len(g.Providers) > 0 {
		return g.Providers
	}
	return g.Sources
}

func LoadProjectConfig(path string) (ProjectConfig, error) {
	if path == "" {
		path = DefaultProjectConfigPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProjectConfig{}, fmt.Errorf("no project config found at %s. Run: envmap init", path)
		}
		return ProjectConfig{}, fmt.Errorf("read project config: %w", err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return ProjectConfig{}, fmt.Errorf("parse project config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return ProjectConfig{}, err
	}
	return cfg, nil
}

func LoadGlobalConfig(path string) (GlobalConfig, error) {
	if path == "" {
		path = DefaultGlobalConfigPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GlobalConfig{}, fmt.Errorf("no global config found at %s. Run: envmap doctor", path)
		}
		return GlobalConfig{}, fmt.Errorf("read global config: %w", err)
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return GlobalConfig{}, fmt.Errorf("parse global config: %w", err)
	}
	if len(cfg.GetProviders()) == 0 {
		return GlobalConfig{}, errors.New("no providers configured in global config; run envmap init or envmap doctor to add one")
	}
	return cfg, nil
}

func (c ProjectConfig) Validate() error {
	if c.Project == "" {
		return errors.New("project config missing project name")
	}
	if len(c.Envs) == 0 {
		return errors.New("project config has no envs defined")
	}
	if c.DefaultEnv == "" {
		return errors.New("project config missing default_env")
	}
	if _, ok := c.Envs[c.DefaultEnv]; !ok {
		return fmt.Errorf("default_env %q not found in envs", c.DefaultEnv)
	}
	return nil
}

func ResolveEnv(cfg ProjectConfig, requested string) (string, error) {
	if requested != "" {
		if _, ok := cfg.Envs[requested]; ok {
			return requested, nil
		}
		return "", fmt.Errorf("unknown env %q; valid envs: %s", requested, joinEnvKeys(cfg))
	}
	return cfg.DefaultEnv, nil
}

func DefaultProjectConfigPath() string {
	return ".envmap.yaml"
}

func DefaultGlobalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(string(os.PathSeparator), "unknown", ".envmap", "config.yaml")
	}
	return filepath.Join(home, ".envmap", "config.yaml")
}

func joinEnvKeys(cfg ProjectConfig) string {
	keys := make([]string, 0, len(cfg.Envs))
	for k := range cfg.Envs {
		keys = append(keys, k)
	}
	return fmt.Sprintf("%v", keys)
}
