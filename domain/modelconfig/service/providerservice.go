// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
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
	// NamespaceForWatchModelConfig returns the namespace identifier used for
	// watching model configuration changes.
	NamespaceForWatchModelConfig() string
}

// ProviderService defines the service for interacting with ModelConfig.
// The provider service is a subset of the ModelConfig service, and is used by
// the provider package to interact with the ModelConfig service. By not
// exposing the full ModelConfig service, the provider package is not able to
// modify the ModelConfig entities, only read them.
type ProviderService struct {
	st ProviderState
}

// NewProviderService creates a new ModelConfig service.
func NewProviderService(
	st ProviderState,
) *ProviderService {
	return &ProviderService{
		st: st,
	}
}

// ModelConfig returns the current config for the model.
func (s *ProviderService) ModelConfig(ctx context.Context) (_ *config.Config, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	stConfig, err := s.st.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("getting model config from state: %w", err)
	}

	altConfig := transform.Map(stConfig, func(k, v string) (string, any) { return k, v })
	return config.New(config.NoDefaults, altConfig)
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
	watcherFactory WatcherFactory,
) *WatchableProviderService {
	return &WatchableProviderService{
		ProviderService: ProviderService{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// Watch returns a watcher that returns keys for any changes to model
// config.
func (s *WatchableProviderService) Watch() (watcher.StringsWatcher, error) {
	// TODO (stickupkid): Wire up trace here. The fallout from this change
	// is quite large.
	return s.watcherFactory.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges(s.st.AllKeysQuery()),
		eventsource.NamespaceFilter(s.st.NamespaceForWatchModelConfig(), changestream.All),
	)
}
