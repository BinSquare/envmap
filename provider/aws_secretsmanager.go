package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

func init() {
	Register(Info{
		Type:           "aws-secretsmanager",
		Description:    "AWS Secrets Manager",
		Factory:        newAWSSecretsManager,
		RequiredFields: []string{"region"},
		OptionalFields: []string{"profile"},
	})
}

type awsSecretsManager struct {
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newAWSSecretsManager(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Region == "" {
		return nil, fmt.Errorf("aws-secretsmanager provider missing region")
	}
	return &awsSecretsManager{envCfg: envCfg, providerCfg: providerCfg}, nil
}

func (p *awsSecretsManager) Get(ctx context.Context, name string) (string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return "", err
	}

	secretName := ApplyPrefix(p.envCfg, name)
	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	})
	if err != nil {
		return "", fmt.Errorf("aws secrets get %s: %w", secretName, err)
	}

	if out.SecretString != nil {
		return aws.ToString(out.SecretString), nil
	}

	if out.SecretBinary != nil {
		return string(out.SecretBinary), nil
	}

	return "", fmt.Errorf("secret %s has no value", secretName)
}

func (p *awsSecretsManager) List(ctx context.Context, prefix string) (map[string]string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]string)
	var nextToken *string

	for {
		input := &secretsmanager.ListSecretsInput{
			NextToken: nextToken,
		}

		if prefix != "" {
			input.Filters = []smtypes.Filter{
				{
					Key:    smtypes.FilterNameStringTypeName,
					Values: []string{prefix},
				},
			}
		}

		out, err := client.ListSecrets(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("aws secrets list: %w", err)
		}

		for _, secret := range out.SecretList {
			secretName := aws.ToString(secret.Name)
			if prefix != "" && !strings.HasPrefix(secretName, prefix) {
				continue
			}

			value, err := p.Get(ctx, TrimPrefix(p.envCfg, secretName))
			if err != nil {
				continue
			}

			var jsonData map[string]string
			if err := json.Unmarshal([]byte(value), &jsonData); err == nil {
				for k, v := range jsonData {
					results[TrimPrefix(p.envCfg, secretName)+"/"+k] = v
				}
			} else {
				results[TrimPrefix(p.envCfg, secretName)] = value
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return results, nil
}

func (p *awsSecretsManager) Set(ctx context.Context, name, value string) error {
	client, err := p.client(ctx)
	if err != nil {
		return err
	}

	secretName := ApplyPrefix(p.envCfg, name)

	_, err = client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(value),
	})
	if err != nil {
		_, createErr := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         aws.String(secretName),
			SecretString: aws.String(value),
		})
		if createErr != nil {
			return fmt.Errorf("aws secrets put %s: %w (create also failed: %v)", secretName, err, createErr)
		}
	}

	return nil
}

func (p *awsSecretsManager) client(ctx context.Context) (*secretsmanager.Client, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(p.providerCfg.Region),
	}
	if p.providerCfg.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(p.providerCfg.Profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	return secretsmanager.NewFromConfig(cfg), nil
}

