package provider

import (
	"context"
	"time"
)

// SecretRecord carries a secret's value plus optional metadata for presentation.
type SecretRecord struct {
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// MetadataLister can return values plus metadata in one call.
type MetadataLister interface {
	ListWithMetadata(ctx context.Context, prefix string) (map[string]SecretRecord, error)
}

// ListOrDescribe fetches secrets with metadata when the provider supports it.
// For providers that do not expose metadata, the map still contains values,
// but CreatedAt is left zero to signal "unknown".
func ListOrDescribe(ctx context.Context, p Provider, prefix string) (map[string]SecretRecord, error) {
	if lister, ok := p.(MetadataLister); ok {
		return lister.ListWithMetadata(ctx, prefix)
	}
	values, err := p.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	records := make(map[string]SecretRecord, len(values))
	for k, v := range values {
		records[k] = SecretRecord{Value: v}
	}
	return records, nil
}
