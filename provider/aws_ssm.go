package provider

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func init() {
	Register(Info{
		Type:           "aws-ssm",
		Description:    "AWS Systems Manager Parameter Store",
		Factory:        newAWSSSM,
		RequiredFields: []string{"region"},
		OptionalFields: []string{"profile"},
	})
}

type awsSSM struct {
	envCfg      EnvConfig
	providerCfg ProviderConfig
}

func newAWSSSM(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error) {
	if providerCfg.Region == "" {
		return nil, fmt.Errorf("aws-ssm provider missing region")
	}
	return &awsSSM{envCfg: envCfg, providerCfg: providerCfg}, nil
}

func (p *awsSSM) Get(ctx context.Context, name string) (string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return "", err
	}
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("aws ssm get %s: %w", name, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("missing secret %s in aws ssm", name)
	}
	return aws.ToString(out.Parameter.Value), nil
}

func (p *awsSSM) List(ctx context.Context, prefix string) (map[string]string, error) {
	client, err := p.client(ctx)
	if err != nil {
		return nil, err
	}
	results := make(map[string]string)
	path := ensurePrefixSlash(prefix)
	if path == "" {
		return nil, fmt.Errorf("aws-ssm requires path_prefix for listing")
	}
	nextToken := (*string)(nil)
	for {
		out, err := client.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
			Path:           aws.String(path),
			WithDecryption: aws.Bool(true),
			Recursive:      aws.Bool(true),
			NextToken:      nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("aws ssm list %s: %w", path, err)
		}
		for _, param := range out.Parameters {
			if param.Name == nil || param.Value == nil {
				continue
			}
			results[aws.ToString(param.Name)] = aws.ToString(param.Value)
		}
		if out.NextToken == nil || aws.ToString(out.NextToken) == "" {
			break
		}
		nextToken = out.NextToken
	}
	return results, nil
}

func (p *awsSSM) Set(ctx context.Context, name, value string) error {
	client, err := p.client(ctx)
	if err != nil {
		return err
	}
	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(name),
		Value:     aws.String(value),
		Type:      types.ParameterTypeSecureString,
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("aws ssm put %s: %w", name, err)
	}
	return nil
}

func (p *awsSSM) client(ctx context.Context) (*ssm.Client, error) {
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
	return ssm.NewFromConfig(cfg), nil
}
