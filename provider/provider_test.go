package provider

import (
	"context"
	"testing"
)

func TestRegistry(t *testing.T) {
	// Registry is already populated by init() functions.
	// Verify core providers are registered.
	required := []string{"aws-ssm", "vault", "local-file", "gcp-secretmanager"}

	for _, typ := range required {
		info, ok := Get(typ)
		if !ok {
			t.Errorf("expected provider %q to be registered", typ)
			continue
		}
		if info.Factory == nil {
			t.Errorf("provider %q has nil factory", typ)
		}
		if info.Description == "" {
			t.Errorf("provider %q has empty description", typ)
		}
	}
}

func TestListTypes(t *testing.T) {
	types := ListTypes()
	if len(types) < 4 {
		t.Errorf("expected at least 4 providers, got %d", len(types))
	}
}

func TestApplyPrefix(t *testing.T) {
	tests := []struct {
		name     string
		cfg      EnvConfig
		key      string
		expected string
	}{
		{
			name:     "path prefix",
			cfg:      EnvConfig{PathPrefix: "/app/dev"},
			key:      "DB_URL",
			expected: "/app/dev/DB_URL",
		},
		{
			name:     "path prefix with trailing slash",
			cfg:      EnvConfig{PathPrefix: "/app/dev/"},
			key:      "DB_URL",
			expected: "/app/dev/DB_URL",
		},
		{
			name:     "prefix without slash",
			cfg:      EnvConfig{Prefix: "myapp_"},
			key:      "DB_URL",
			expected: "myapp_DB_URL",
		},
		{
			name:     "path prefix takes precedence",
			cfg:      EnvConfig{PathPrefix: "/app/", Prefix: "ignored_"},
			key:      "KEY",
			expected: "/app/KEY",
		},
		{
			name:     "no prefix",
			cfg:      EnvConfig{},
			key:      "RAW_KEY",
			expected: "RAW_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyPrefix(tt.cfg, tt.key)
			if got != tt.expected {
				t.Errorf("ApplyPrefix() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTrimPrefix(t *testing.T) {
	tests := []struct {
		name     string
		cfg      EnvConfig
		input    string
		expected string
	}{
		{
			name:     "trim path prefix",
			cfg:      EnvConfig{PathPrefix: "/app/dev"},
			input:    "/app/dev/DB_URL",
			expected: "DB_URL",
		},
		{
			name:     "trim prefix",
			cfg:      EnvConfig{Prefix: "myapp_"},
			input:    "myapp_DB_URL",
			expected: "DB_URL",
		},
		{
			name:     "no match returns input",
			cfg:      EnvConfig{PathPrefix: "/other/"},
			input:    "/app/dev/KEY",
			expected: "/app/dev/KEY",
		},
		{
			name:     "no prefix configured",
			cfg:      EnvConfig{},
			input:    "KEY",
			expected: "KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimPrefix(tt.cfg, tt.input)
			if got != tt.expected {
				t.Errorf("TrimPrefix() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestApplyTrimRoundtrip(t *testing.T) {
	cfg := EnvConfig{PathPrefix: "/myapp/prod/"}
	key := "DATABASE_URL"

	applied := ApplyPrefix(cfg, key)
	trimmed := TrimPrefix(cfg, applied)

	if trimmed != key {
		t.Errorf("roundtrip failed: started with %q, got %q", key, trimmed)
	}
}

// mockProvider for testing factory registration pattern
type mockProvider struct{}

func (m *mockProvider) Get(ctx context.Context, name string) (string, error) {
	return "mock", nil
}
func (m *mockProvider) List(ctx context.Context, prefix string) (map[string]string, error) {
	return nil, nil
}
func (m *mockProvider) Set(ctx context.Context, name, value string) error {
	return nil
}

func TestFactoryPattern(t *testing.T) {
	// Verify factory returns correct interface type
	info, ok := Get("local-file")
	if !ok {
		t.Skip("local-file provider not registered")
	}

	// Factory should fail gracefully with missing config
	_, err := info.Factory(EnvConfig{}, ProviderConfig{Type: "local-file"})
	if err == nil {
		t.Error("expected error for missing required config")
	}
}

