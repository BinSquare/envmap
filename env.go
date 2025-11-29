package main

import (
	"context"
	"fmt"

	"github.com/binsquare/envmap/provider"
)

func NewProvider(envName string, envCfg EnvConfig, globalCfg GlobalConfig) (provider.Provider, error) {
	providerName := envCfg.GetProvider()
	if providerName == "" {
		return nil, fmt.Errorf("env %q missing provider in .envmap.yaml", envName)
	}

	providerCfg, ok := globalCfg.GetProviders()[providerName]
	if !ok {
		return nil, fmt.Errorf("no provider named %q configured in %s. Run: envmap doctor", providerName, DefaultGlobalConfigPath())
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
