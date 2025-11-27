package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

func init() {
	Register(Info{
		Type:           "vault",
		Description:    "HashiCorp Vault",
		Factory:        newVault,
		RequiredFields: []string{"address"},
		OptionalFields: []string{"token", "mount", "namespace"},
	})
}

type vaultProvider struct {
	client      *vault.Client
	mount       string
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newVault(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Extra == nil {
		providerCfg.Extra = map[string]any{}
	}

	address, ok := providerCfg.Extra["address"].(string)
	if !ok || address == "" {
		return nil, fmt.Errorf("vault provider requires address in config")
	}

	config := vault.DefaultConfig()
	config.Address = address

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("init vault client: %w", err)
	}

	token := os.Getenv("VAULT_TOKEN")
	if t, ok := providerCfg.Extra["token"].(string); ok && t != "" {
		token = t
	}
	if token != "" {
		client.SetToken(token)
	}

	if ns, ok := providerCfg.Extra["namespace"].(string); ok && ns != "" {
		client.SetNamespace(ns)
	}

	mount := "secret"
	if m, ok := providerCfg.Extra["mount"].(string); ok && m != "" {
		mount = m
	}

	return &vaultProvider{
		client:      client,
		mount:       mount,
		envCfg:      envCfg,
		providerCfg: providerCfg,
	}, nil
}

func (p *vaultProvider) secretPath(name string) string {
	prefixed := ApplyPrefix(p.envCfg, name)
	return fmt.Sprintf("%s/data/%s", p.mount, prefixed)
}

func (p *vaultProvider) Get(ctx context.Context, name string) (string, error) {
	path := p.secretPath(name)
	secret, err := p.client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		return "", fmt.Errorf("vault get %s: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("secret %s not found in vault", path)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault secret %s has unexpected format", path)
	}

	value, ok := data["value"].(string)
	if !ok {
		return "", fmt.Errorf("vault secret %s missing 'value' field", path)
	}
	return value, nil
}

func (p *vaultProvider) List(ctx context.Context, prefix string) (map[string]string, error) {
	listPath := fmt.Sprintf("%s/metadata/%s", p.mount, ensurePrefixSlash(prefix))
	secret, err := p.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, fmt.Errorf("vault list %s: %w", listPath, err)
	}

	out := make(map[string]string)
	if secret == nil || secret.Data == nil {
		return out, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return out, nil
	}

	for _, k := range keys {
		keyStr, ok := k.(string)
		if !ok || strings.HasSuffix(keyStr, "/") {
			continue
		}

		fullPath := prefix + keyStr
		value, err := p.Get(ctx, TrimPrefix(p.envCfg, fullPath))
		if err != nil {
			continue
		}
		out[TrimPrefix(p.envCfg, fullPath)] = value
	}
	return out, nil
}

func (p *vaultProvider) Set(ctx context.Context, name, value string) error {
	path := p.secretPath(name)
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"value": value,
		},
	}
	_, err := p.client.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return fmt.Errorf("vault put %s: %w", path, err)
	}
	return nil
}
