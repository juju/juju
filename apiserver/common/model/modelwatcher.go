// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ModelConfigWatcher implements two common methods for use by various
// facades - WatchForModelConfigChanges and ModelConfig.
type ModelConfigWatcher struct {
	modelConfigService ModelConfigService
	watcherRegistry    facade.WatcherRegistry
}

// NewModelConfigWatcher returns a new ModelConfigWatcher. Active watchers
// will be stored in the provided facade.WatcherRegistry.
func NewModelConfigWatcher(modelConfigService ModelConfigService, watcherRegistry facade.WatcherRegistry) *ModelConfigWatcher {
	return &ModelConfigWatcher{
		modelConfigService: modelConfigService,
		watcherRegistry:    watcherRegistry,
	}
}

// WatchForModelConfigChanges returns a NotifyWatcher that observes
// changes to the model configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (m *ModelConfigWatcher) WatchForModelConfigChanges(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w, err := m.modelConfigService.Watch(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	notifyWatcher, err := watcher.Normalise[[]string](w)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](ctx, m.watcherRegistry, notifyWatcher)
	return result, err
}

// ModelConfig returns the current model's configuration.
func (m *ModelConfigWatcher) ModelConfig(ctx context.Context) (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}
	config, err := m.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}
