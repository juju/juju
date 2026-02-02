// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// ProviderState defines the state methods required by the ProviderService.
type ProviderState interface {
	// AllKeysQuery returns a SQL statement that will return all known model config
	// keys.
	AllKeysQuery() string

	// ModelConfig returns the currently set config for the model.
	ModelConfig(context.Context) (map[string]string, error)

	// NamespacesForWatchModelConfig returns the namespace identifiers used for
	// watching model configuration changes.
	NamespacesForWatchModelConfig() []string

	// GetModelAgentVersionAndStream returns the current model's set agent
	// version and stream.
	GetModelAgentVersionAndStream(context.Context) (ver string, stream string, err error)
}

// ProviderService defines the service for interacting with ModelConfig.
// The provider service is a subset of the ModelConfig service, and is used by
// the provider package to interact with the ModelConfig service. By not
// exposing the full ModelConfig service, the provider package is not able to
// modify the ModelConfig entities, only read them.
//
// Provider-specific config attributes are stored as strings in the database
// (map[string]string). When reading from the database, if a providerSchema is
// provided, the service will coerce provider-specific attributes from strings
// to their proper types (bool, int, etc.) according to the provider's schema.
// This ensures that provider code can safely type-assert these values without
// panicking.
type ProviderService struct {
	st                            ProviderState
	modelConfigProviderGetterFunc ModelConfigProviderFunc
}

// NewProviderService creates a new ModelConfig service.
func NewProviderService(
	st ProviderState,
	modelConfigProviderGetterFunc ModelConfigProviderFunc,
) *ProviderService {
	return &ProviderService{
		st:                            st,
		modelConfigProviderGetterFunc: modelConfigProviderGetterFunc,
	}
}

// ModelConfig returns the current config for the model.
func (s *ProviderService) ModelConfig(ctx context.Context) (*config.Config, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	stConfig, err := s.st.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model config from state: %w", err)
	}

	// Coerce provider-specific attributes from string to their proper types.
	altConfig, err := s.getCoercedProviderConfig(ctx, stConfig)
	if err != nil {
		return nil, errors.Errorf("coercing provider config attributes: %w", err)
	}
	return config.New(config.NoDefaults, altConfig)
}

// getCoercedProviderConfig converts a map[string]string from the database to
// map[string]any and coerces any provider-specific values that are found in the
// provider's schema. This is necessary because the database stores all config
// as strings, but provider code expects typed values (e.g., bool, int) for
// provider-specific attributes.
func (s *ProviderService) getCoercedProviderConfig(ctx context.Context, m map[string]string) (map[string]any, error) {
	if s.modelConfigProviderGetterFunc == nil {
		return nil, errors.New("no model config provider getter")
	}

	cloudType, ok := m[config.TypeKey]
	if !ok || cloudType == "" {
		// No cloud type - just convert without coercion.
		return stringMapToAny(m), nil
	}

	provider, err := s.modelConfigProviderGetterFunc(ctx, cloudType)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Capture(err)
	} else if provider == nil {
		// Provider not found or doesn't support config schema.
		return nil, errors.New("provider not found or doesn't support config schema")
	}

	fields := provider.ConfigSchema()

	result := make(map[string]any, len(m))
	for key, strVal := range m {
		if field, ok := fields[key]; ok {
			// This is a provider-specific attribute - coerce it to proper type.
			coercedVal, err := field.Coerce(strVal, []string{key})
			if err != nil {
				return nil, errors.Errorf("coercing provider config key %q: %w", key, err)
			}
			result[key] = coercedVal
			continue
		}

		// Not a provider-specific attribute - keep as string.
		result[key] = strVal
	}

	return result, nil
}

// WatchableProviderService defines the service for interacting with ModelConfig
// and the ability to create watchers.
type WatchableProviderService struct {
	ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableProviderService creates a new WatchableProviderService for
// interacting with ModelConfig and the ability to create watchers.
func NewWatchableProviderService(
	st ProviderState,
	modelConfigProviderGetterFunc ModelConfigProviderFunc,
	watcherFactory WatcherFactory,
) *WatchableProviderService {
	return &WatchableProviderService{
		ProviderService: ProviderService{
			st:                            st,
			modelConfigProviderGetterFunc: modelConfigProviderGetterFunc,
		},
		watcherFactory: watcherFactory,
	}
}

// Watch returns a watcher that returns keys for any changes to model
// config.
func (s *WatchableProviderService) Watch(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespaces := s.st.NamespacesForWatchModelConfig()
	if len(namespaces) == 0 {
		return nil, errors.Errorf("no namespaces for watching model config")
	}

	filters := transform.Slice(namespaces, func(ns string) eventsource.FilterOption {
		return eventsource.NamespaceFilter(ns, changestream.All)
	})

	agentVersion, agentStream, err := s.st.GetModelAgentVersionAndStream(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model agent version and stream: %w", err)
	}

	return s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(s.st.AllKeysQuery()),
		"model config watcher",
		modelConfigMapper(s.st, agentVersion, agentStream),
		filters[0], filters[1:]...,
	)
}
