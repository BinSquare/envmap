package provider

import "strings"

// EnvConfig represents the environment-specific configuration from the project file.
type EnvConfig struct {
	Provider   string `yaml:"provider"`
	PathPrefix string `yaml:"path_prefix"`
	Prefix     string `yaml:"prefix"`
}

// ProviderConfig represents the provider configuration from the global config file.
type ProviderConfig struct {
	Type       string            `yaml:"type"`
	Profile    string            `yaml:"profile,omitempty"`
	Region     string            `yaml:"region,omitempty"`
	Path       string            `yaml:"path,omitempty"`
	Encryption *EncryptionConfig `yaml:"encryption,omitempty"`
	Extra      map[string]any    `yaml:",inline"`
}

// EncryptionConfig holds encryption settings for local file storage.
type EncryptionConfig struct {
	Type    string `yaml:"type"`
	KeyFile string `yaml:"key_file,omitempty"`
	KeyEnv  string `yaml:"key_env,omitempty"`
}

// ApplyPrefix builds the fully-qualified secret name for a given key.
func ApplyPrefix(envCfg EnvConfig, key string) string {
	if envCfg.PathPrefix != "" {
		return ensureTrailingSlash(envCfg.PathPrefix) + key
	}
	if envCfg.Prefix != "" {
		return envCfg.Prefix + key
	}
	return key
}

// TrimPrefix removes the configured prefix from a secret name when presenting to the user.
func TrimPrefix(envCfg EnvConfig, name string) string {
	if envCfg.PathPrefix != "" {
		p := ensureTrailingSlash(envCfg.PathPrefix)
		return strings.TrimPrefix(name, p)
	}
	if envCfg.Prefix != "" {
		return strings.TrimPrefix(name, envCfg.Prefix)
	}
	return name
}

// ResolvedPrefix returns the configured prefix (path_prefix or prefix) in its normalized form.
func ResolvedPrefix(envCfg EnvConfig) string {
	if envCfg.PathPrefix != "" {
		return ensureTrailingSlash(envCfg.PathPrefix)
	}
	return envCfg.Prefix
}

func ensureTrailingSlash(prefix string) string {
	if strings.HasSuffix(prefix, "/") {
		return prefix
	}
	return prefix + "/"
}

// ensurePrefixSlash ensures the prefix ends with a slash if non-empty.
// This is used by providers that use path-based hierarchies (e.g., AWS SSM).
func ensurePrefixSlash(prefix string) string {
	if prefix == "" {
		return ""
	}
	if strings.HasSuffix(prefix, "/") {
		return prefix
	}
	return prefix + "/"
}

