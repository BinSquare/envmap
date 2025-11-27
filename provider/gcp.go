package provider

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"
)

func init() {
	Register(Info{
		Type:           "gcp-secretmanager",
		Description:    "Google Cloud Secret Manager",
		Factory:        newGCPSecretManager,
		RequiredFields: []string{"project"},
		OptionalFields: []string{"credentials_file"},
	})
}

type gcpSecretManager struct {
	svc         *secretmanager.Service
	projectID   string
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newGCPSecretManager(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Extra == nil {
		providerCfg.Extra = map[string]any{}
	}
	project, ok := providerCfg.Extra["project"].(string)
	if !ok || project == "" {
		return nil, fmt.Errorf("gcp-secretmanager provider requires project in config")
	}
	opts := []option.ClientOption{}
	if credFile, ok := providerCfg.Extra["credentials_file"].(string); ok && credFile != "" {
		opts = append(opts, option.WithCredentialsFile(credFile))
	}
	svc, err := secretmanager.NewService(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("init gcp secret manager: %w", err)
	}
	return &gcpSecretManager{
		svc:         svc,
		projectID:   project,
		envCfg:      envCfg,
		providerCfg: providerCfg,
	}, nil
}

func (p *gcpSecretManager) secretName(key string) string {
	name := ApplyPrefix(p.envCfg, key)
	return fmt.Sprintf("projects/%s/secrets/%s", p.projectID, name)
}

func (p *gcpSecretManager) Get(ctx context.Context, name string) (string, error) {
	secretName := p.secretName(name) + "/versions/latest"
	resp, err := p.svc.Projects.Secrets.Versions.Access(secretName).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gcp secret get %s: %w", secretName, err)
	}
	if resp.Payload == nil || resp.Payload.Data == "" {
		return "", fmt.Errorf("secret %s has no data", secretName)
	}
	data, err := base64.StdEncoding.DecodeString(resp.Payload.Data)
	if err != nil {
		return "", fmt.Errorf("decode secret %s: %w", secretName, err)
	}
	return string(data), nil
}

func (p *gcpSecretManager) List(ctx context.Context, prefix string) (map[string]string, error) {
	out := make(map[string]string)
	parent := fmt.Sprintf("projects/%s", p.projectID)
	req := p.svc.Projects.Secrets.List(parent)
	if prefix != "" {
		req = req.Filter(fmt.Sprintf("name:%s", prefix))
	}
	if err := req.Pages(ctx, func(page *secretmanager.ListSecretsResponse) error {
		for _, sec := range page.Secrets {
			name := sec.Name[strings.LastIndex(sec.Name, "/")+1:]
			fullName := p.secretName(TrimPrefix(p.envCfg, name)) + "/versions/latest"
			resp, err := p.svc.Projects.Secrets.Versions.Access(fullName).Context(ctx).Do()
			if err != nil || resp.Payload == nil {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(resp.Payload.Data)
			if err != nil {
				continue
			}
			out[TrimPrefix(p.envCfg, name)] = string(decoded)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("gcp secret list: %w", err)
	}
	return out, nil
}

func (p *gcpSecretManager) Set(ctx context.Context, name, value string) error {
	secretName := p.secretName(name)
	if _, err := p.svc.Projects.Secrets.Get(secretName).Context(ctx).Do(); err != nil {
		if _, err := p.svc.Projects.Secrets.Create(fmt.Sprintf("projects/%s", p.projectID), &secretmanager.Secret{
			Replication: &secretmanager.Replication{Automatic: &secretmanager.Automatic{}},
			Name:        secretName,
		}).SecretId(ApplyPrefix(p.envCfg, name)).Context(ctx).Do(); err != nil {
			return fmt.Errorf("gcp secret create %s: %w", secretName, err)
		}
	}
	_, err := p.svc.Projects.Secrets.AddVersion(secretName, &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{
			Data: base64.StdEncoding.EncodeToString([]byte(value)),
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("gcp secret add version %s: %w", secretName, err)
	}
	return nil
}

