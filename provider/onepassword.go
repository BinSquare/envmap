package provider

import (
	"context"
	"fmt"
	"os"
	"strings"

	opconnect "github.com/1Password/connect-sdk-go/connect"
	"github.com/1Password/connect-sdk-go/onepassword"
)

func init() {
	Register(Info{
		Type:           "onepassword",
		Description:    "1Password Connect server",
		Factory:        newOnePassword,
		RequiredFields: []string{"connect_host"},
		OptionalFields: []string{"connect_token", "vault_id", "vault"},
	})
}

type onePassword struct {
	client      opconnect.Client
	vaultID     string
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newOnePassword(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Extra == nil {
		providerCfg.Extra = map[string]any{}
	}
	rawHost, ok := providerCfg.Extra["connect_host"].(string)
	if !ok || rawHost == "" {
		return nil, fmt.Errorf("onepassword provider requires connect_host")
	}
	token := os.Getenv("OP_CONNECT_TOKEN")
	if t, ok := providerCfg.Extra["connect_token"].(string); ok && t != "" {
		token = t
	}
	if token == "" {
		return nil, fmt.Errorf("onepassword provider requires OP_CONNECT_TOKEN env or connect_token in config")
	}
	client := opconnect.NewClient(rawHost, token)
	var vaultID string
	if v, ok := providerCfg.Extra["vault_id"].(string); ok && v != "" {
		vaultID = v
	} else if v, ok := providerCfg.Extra["vault"].(string); ok && v != "" {
		vault, err := client.GetVaultByTitle(v)
		if err != nil {
			return nil, fmt.Errorf("resolve 1password vault %s: %w", v, err)
		}
		vaultID = vault.ID
	} else {
		vault, err := client.GetVaultByTitle("Private")
		if err != nil {
			return nil, fmt.Errorf("onepassword provider missing vault; set vault_id or vault name in config")
		}
		vaultID = vault.ID
	}
	return &onePassword{
		client:      client,
		vaultID:     vaultID,
		envCfg:      envCfg,
		providerCfg: providerCfg,
	}, nil
}

func (p *onePassword) Get(ctx context.Context, name string) (string, error) {
	itemName := ApplyPrefix(p.envCfg, name)
	item, err := p.client.GetItemByTitle(itemName, p.vaultID)
	if err != nil {
		return "", fmt.Errorf("1password get %s: %w", itemName, err)
	}
	for _, f := range item.Fields {
		if f.Label == "value" || f.Purpose == "PASSWORD" {
			return fmt.Sprintf("%v", f.Value), nil
		}
	}
	return "", fmt.Errorf("1password item %s has no usable fields", itemName)
}

func (p *onePassword) List(ctx context.Context, prefix string) (map[string]string, error) {
	items, err := p.client.GetItems(p.vaultID)
	if err != nil {
		return nil, fmt.Errorf("1password list: %w", err)
	}
	out := make(map[string]string)
	for _, item := range items {
		name := item.Title
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		full, err := p.client.GetItem(item.ID, p.vaultID)
		if err != nil {
			continue
		}
		for _, f := range full.Fields {
			if f.Label == "value" || f.Purpose == "PASSWORD" {
				out[TrimPrefix(p.envCfg, name)] = fmt.Sprintf("%v", f.Value)
				break
			}
		}
	}
	return out, nil
}

func (p *onePassword) Set(ctx context.Context, name, value string) error {
	itemName := ApplyPrefix(p.envCfg, name)
	item := onepassword.Item{
		Title: itemName,
		Vault: onepassword.ItemVault{ID: p.vaultID},
		Fields: []*onepassword.ItemField{
			{
				Label:   "value",
				Type:    "STRING",
				Value:   value,
				Purpose: "PASSWORD",
			},
		},
	}
	if _, err := p.client.GetItemByTitle(itemName, p.vaultID); err == nil {
		_, err = p.client.UpdateItem(&item, p.vaultID)
		if err != nil {
			return fmt.Errorf("1password update %s: %w", itemName, err)
		}
		return nil
	}
	_, err := p.client.CreateItem(&item, p.vaultID)
	if err != nil {
		return fmt.Errorf("1password create %s: %w", itemName, err)
	}
	return nil
}
