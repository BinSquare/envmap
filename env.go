package main

import (
	"context"
	"fmt"
	"os"

	"github.com/binsquare/envmap/provider"
)

func NewProvider(envName string, envCfg EnvConfig, globalCfg GlobalConfig) (provider.Provider, error) {
	providerName := envCfg.GetProvider()
	if providerName == "" {
		return nil, fmt.Errorf("env %q missing provider in .envmap.yaml", envName)
	}

	providers := globalCfg.GetProviders()
	providerCfg, ok := providers[providerName]
	if !ok {
		// If there is exactly one provider configured, fall back to it to avoid mismatch pain.
		if len(providers) == 1 {
			for name, cfg := range providers {
				fmt.Fprintf(os.Stderr, "warning: provider %q not found; using configured provider %q\n", providerName, name)
				providerName = name
				providerCfg = cfg
				ok = true
				break
			}
		}
		if !ok {
			avail := make([]string, 0, len(providers))
			for k := range providers {
				avail = append(avail, k)
			}
			return nil, fmt.Errorf("no provider named %q configured in %s. Available: %v.", providerName, DefaultGlobalConfigPath(), avail)
		}
	}

	info, ok := provider.Get(providerCfg.Type)
	if !ok {
		return nil, fmt.Errorf("unknown provider type %q for provider %q. Available: %v", providerCfg.Type, providerName, provider.ListTypes())
	}

	return info.Factory(envCfg.ToProviderConfig(), providerCfg)
}

func CollectEnv(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName string) (map[string]string, error) {
	records, err := CollectEnvWithMetadata(ctx, projectCfg, globalCfg, envName)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(records))
	for k, rec := range records {
		out[k] = rec.Value
	}
	return out, nil
}

func CollectEnvWithMetadata(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName string) (map[string]provider.SecretRecord, error) {
	envCfg, ok := projectCfg.Envs[envName]
	if !ok {
		return nil, fmt.Errorf("env %q not found in project config", envName)
	}
	p, err := NewProvider(envName, envCfg, globalCfg)
	if err != nil {
		return nil, err
	}
	return provider.ListOrDescribe(ctx, p, provider.ResolvedPrefix(envCfg.ToProviderConfig()))
}

func FetchSecret(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName, key string) (string, error) {
	envCfg, ok := projectCfg.Envs[envName]
	if !ok {
		return "", fmt.Errorf("env %q not found in project config", envName)
	}
	p, err := NewProvider(envName, envCfg, globalCfg)
	if err != nil {
		return "", err
	}
	return p.Get(ctx, provider.ApplyPrefix(envCfg.ToProviderConfig(), key))
}

func WriteSecret(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName, key, value string) error {
	envCfg, ok := projectCfg.Envs[envName]
	if !ok {
		return fmt.Errorf("env %q not found in project config", envName)
	}
	p, err := NewProvider(envName, envCfg, globalCfg)
	if err != nil {
		return err
	}
	return p.Set(ctx, provider.ApplyPrefix(envCfg.ToProviderConfig(), key), value)
}

func DeleteSecret(ctx context.Context, projectCfg ProjectConfig, globalCfg GlobalConfig, envName, key string) error {
	envCfg, ok := projectCfg.Envs[envName]
	if !ok {
		return fmt.Errorf("env %q not found in project config", envName)
	}
	p, err := NewProvider(envName, envCfg, globalCfg)
	if err != nil {
		return err
	}
	if deleter, ok := p.(interface {
		Delete(ctx context.Context, name string) error
	}); ok {
		return deleter.Delete(ctx, provider.ApplyPrefix(envCfg.ToProviderConfig(), key))
	}
	return fmt.Errorf("provider %s does not support delete", envCfg.GetProvider())
}
