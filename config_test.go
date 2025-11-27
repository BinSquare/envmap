package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/binsquare/envmap/provider"
)

func TestEnvConfigGetProvider(t *testing.T) {
	tests := []struct {
		name     string
		cfg      EnvConfig
		expected string
	}{
		{
			name:     "new provider field",
			cfg:      EnvConfig{Provider: "vault-prod"},
			expected: "vault-prod",
		},
		{
			name:     "legacy source field",
			cfg:      EnvConfig{Source: "aws-ssm"},
			expected: "aws-ssm",
		},
		{
			name:     "provider takes precedence",
			cfg:      EnvConfig{Provider: "new", Source: "old"},
			expected: "new",
		},
		{
			name:     "both empty",
			cfg:      EnvConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.GetProvider()
			if got != tt.expected {
				t.Errorf("GetProvider() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGlobalConfigGetProviders(t *testing.T) {
	t.Run("new providers field", func(t *testing.T) {
		cfg := GlobalConfig{
			Providers: map[string]provider.ProviderConfig{
				"vault": {Type: "vault"},
			},
		}
		providers := cfg.GetProviders()
		if len(providers) != 1 {
			t.Errorf("expected 1 provider, got %d", len(providers))
		}
		if _, ok := providers["vault"]; !ok {
			t.Error("expected 'vault' provider")
		}
	})

	t.Run("legacy sources field", func(t *testing.T) {
		cfg := GlobalConfig{
			Sources: map[string]provider.ProviderConfig{
				"aws": {Type: "aws-ssm"},
			},
		}
		providers := cfg.GetProviders()
		if len(providers) != 1 {
			t.Errorf("expected 1 provider, got %d", len(providers))
		}
		if _, ok := providers["aws"]; !ok {
			t.Error("expected 'aws' provider")
		}
	})

	t.Run("providers takes precedence", func(t *testing.T) {
		cfg := GlobalConfig{
			Providers: map[string]provider.ProviderConfig{
				"new": {Type: "vault"},
			},
			Sources: map[string]provider.ProviderConfig{
				"old": {Type: "aws-ssm"},
			},
		}
		providers := cfg.GetProviders()
		if _, ok := providers["new"]; !ok {
			t.Error("expected 'new' provider from Providers field")
		}
		if _, ok := providers["old"]; ok {
			t.Error("Sources should be ignored when Providers is set")
		}
	})
}

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".envmap.yaml")

	content := `
project: testapp
default_env: dev
envs:
  dev:
    provider: local
    path_prefix: /test/
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProjectConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadProjectConfig: %v", err)
	}

	if cfg.Project != "testapp" {
		t.Errorf("Project = %q, want %q", cfg.Project, "testapp")
	}
	if cfg.DefaultEnv != "dev" {
		t.Errorf("DefaultEnv = %q, want %q", cfg.DefaultEnv, "dev")
	}
	if len(cfg.Envs) != 1 {
		t.Errorf("len(Envs) = %d, want 1", len(cfg.Envs))
	}
}

func TestLoadProjectConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "missing project",
			content: "default_env: dev\nenvs:\n  dev:\n    provider: x",
			wantErr: true,
		},
		{
			name:    "missing envs",
			content: "project: x\ndefault_env: dev",
			wantErr: true,
		},
		{
			name:    "missing default_env",
			content: "project: x\nenvs:\n  dev:\n    provider: y",
			wantErr: true,
		},
		{
			name:    "default_env not in envs",
			content: "project: x\ndefault_env: prod\nenvs:\n  dev:\n    provider: y",
			wantErr: true,
		},
		{
			name:    "valid config",
			content: "project: x\ndefault_env: dev\nenvs:\n  dev:\n    provider: y",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".envmap.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadProjectConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveEnv(t *testing.T) {
	cfg := ProjectConfig{
		DefaultEnv: "dev",
		Envs: map[string]EnvConfig{
			"dev":  {},
			"prod": {},
		},
	}

	tests := []struct {
		requested string
		expected  string
		wantErr   bool
	}{
		{"", "dev", false},
		{"dev", "dev", false},
		{"prod", "prod", false},
		{"staging", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.requested, func(t *testing.T) {
			got, err := ResolveEnv(cfg, tt.requested)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.expected {
				t.Errorf("ResolveEnv() = %q, want %q", got, tt.expected)
			}
		})
	}
}
