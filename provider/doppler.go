package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

func init() {
	Register(Info{
		Type:           "doppler",
		Description:    "Doppler SecretOps Platform",
		Factory:        newDoppler,
		RequiredFields: []string{"project", "config"},
		OptionalFields: []string{"token"},
	})
}

const dopplerAPIBase = "https://api.doppler.com/v3"

type doppler struct {
	token       string
	project     string
	config      string
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newDoppler(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Extra == nil {
		providerCfg.Extra = map[string]any{}
	}

	project, ok := providerCfg.Extra["project"].(string)
	if !ok || project == "" {
		return nil, fmt.Errorf("doppler provider requires project in config")
	}

	cfg, ok := providerCfg.Extra["config"].(string)
	if !ok || cfg == "" {
		return nil, fmt.Errorf("doppler provider requires config in config")
	}

	token := os.Getenv("DOPPLER_TOKEN")
	if t, ok := providerCfg.Extra["token"].(string); ok && t != "" {
		token = t
	}
	if token == "" {
		return nil, fmt.Errorf("doppler provider requires DOPPLER_TOKEN env or token in config")
	}

	return &doppler{
		token:       token,
		project:     project,
		config:      cfg,
		envCfg:      envCfg,
		providerCfg: providerCfg,
	}, nil
}

func (p *doppler) doRequest(ctx context.Context, method, path string) (*http.Response, error) {
	url := dopplerAPIBase + path

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(p.token, "")
	req.Header.Set("Accept", "application/json")

	return http.DefaultClient.Do(req)
}

func (p *doppler) Get(ctx context.Context, name string) (string, error) {
	secrets, err := p.List(ctx, "")
	if err != nil {
		return "", err
	}

	key := ApplyPrefix(p.envCfg, name)
	value, ok := secrets[TrimPrefix(p.envCfg, key)]
	if !ok {
		return "", fmt.Errorf("secret %s not found in doppler", key)
	}
	return value, nil
}

func (p *doppler) List(ctx context.Context, prefix string) (map[string]string, error) {
	path := fmt.Sprintf("/configs/config/secrets?project=%s&config=%s", p.project, p.config)

	resp, err := p.doRequest(ctx, "GET", path)
	if err != nil {
		return nil, fmt.Errorf("doppler list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doppler list returned status %d", resp.StatusCode)
	}

	var result struct {
		Secrets map[string]struct {
			Raw string `json:"raw"`
		} `json:"secrets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("doppler parse response: %w", err)
	}

	out := make(map[string]string)
	for k, v := range result.Secrets {
		trimmed := TrimPrefix(p.envCfg, k)
		if prefix != "" && len(trimmed) < len(prefix) {
			continue
		}
		out[trimmed] = v.Raw
	}
	return out, nil
}

func (p *doppler) Set(ctx context.Context, name, value string) error {
	return fmt.Errorf("doppler Set not implemented: use doppler CLI to set secrets")
}
