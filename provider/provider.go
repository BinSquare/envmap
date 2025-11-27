// Package provider defines the interface and registry for secret providers.
//
// To add a new provider:
// 1. Create a new file in this package (e.g., myprovider.go)
// 2. Implement the Provider interface
// 3. Register it in an init() function using Register()
package provider

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNotConfigured  = errors.New("provider not configured")
	ErrNotImplemented = errors.New("provider not implemented")
)

// Provider defines the interface for all secret backends.
type Provider interface {
	// Get retrieves a single secret by name.
	Get(ctx context.Context, name string) (string, error)
	// List returns all secrets matching the given prefix.
	List(ctx context.Context, prefix string) (map[string]string, error)
	// Set creates or updates a secret.
	Set(ctx context.Context, name, value string) error
}

// Factory creates a Provider from configuration.
type Factory func(envCfg EnvConfig, providerCfg ProviderConfig) (Provider, error)

// Info contains metadata about a registered provider type.
type Info struct {
	// Type is the unique identifier for this provider (e.g., "aws-ssm", "vault").
	Type string
	// Description provides a human-readable description of the provider.
	Description string
	// Factory creates instances of this provider type.
	Factory Factory
	// RequiredFields lists the configuration fields required for this provider.
	RequiredFields []string
	// OptionalFields lists optional configuration fields.
	OptionalFields []string
}

// registry holds all registered provider types.
type registry struct {
	mu        sync.RWMutex
	providers map[string]Info
}

var globalRegistry = &registry{
	providers: make(map[string]Info),
}

// Register registers a new provider type with the registry.
// This should be called from init() functions in provider implementation files.
func Register(info Info) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if info.Type == "" {
		panic("provider type cannot be empty")
	}
	if info.Factory == nil {
		panic("provider factory cannot be nil")
	}
	if _, exists := globalRegistry.providers[info.Type]; exists {
		panic(fmt.Sprintf("provider type %q already registered", info.Type))
	}
	globalRegistry.providers[info.Type] = info
}

// Get returns information about a registered provider type.
func Get(providerType string) (Info, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	info, ok := globalRegistry.providers[providerType]
	return info, ok
}

// List returns all registered provider types.
func List() []Info {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	types := make([]Info, 0, len(globalRegistry.providers))
	for _, info := range globalRegistry.providers {
		types = append(types, info)
	}
	return types
}

// ListTypes returns just the type names of all registered providers.
func ListTypes() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	types := make([]string, 0, len(globalRegistry.providers))
	for t := range globalRegistry.providers {
		types = append(types, t)
	}
	return types
}
