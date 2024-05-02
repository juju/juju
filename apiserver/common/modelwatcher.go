// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statewatcher "github.com/juju/juju/state/watcher"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch() (watcher.StringsWatcher, error)
}

// ModelWatcher implements two common methods for use by various
// facades - WatchForModelConfigChanges and ModelConfig.
type ModelWatcher struct {
	modelConfigService ModelConfigService
	watcherRegistry    facade.WatcherRegistry
}

// NewModelWatcher returns a new ModelWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, model tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current model".
func NewModelWatcher(modelConfigService ModelConfigService, watcherRegistry facade.WatcherRegistry) *ModelWatcher {
	return &ModelWatcher{
		modelConfigService: modelConfigService,
		watcherRegistry:    watcherRegistry,
	}
}

// WatchForModelConfigChanges returns a NotifyWatcher that observes
// changes to the model configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (m *ModelWatcher) WatchForModelConfigChanges(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w, err := m.modelConfigService.Watch()
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
func (m *ModelWatcher) ModelConfig(ctx context.Context) (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}
	config, err := m.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}

// MongoModelWatcher implements two common methods for use by various
// facades - WatchForModelConfigChanges and ModelConfig.
type MongoModelWatcher struct {
	st        state.ModelAccessor
	resources facade.Resources
}

// NewMongoModelWatcher returns a new MongoModelWatcher. Active watchers
// will be stored in the provided Resources. The two GetAuthFunc
// callbacks will be used on each invocation of the methods to
// determine current permissions.
// Right now, model tags are not used, so both created AuthFuncs
// are called with "" for tag, which means "the current model".
func NewMongoModelWatcher(st state.ModelAccessor, resources facade.Resources) *MongoModelWatcher {
	return &MongoModelWatcher{
		st:        st,
		resources: resources,
	}
}

// WatchForModelConfigChanges returns a NotifyWatcher that observes
// changes to the model configuration.
// Note that although the NotifyWatchResult contains an Error field,
// it's not used because we are only returning a single watcher,
// so we use the regular error return.
func (m *MongoModelWatcher) WatchForModelConfigChanges(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	watch := m.st.WatchForModelConfigChanges()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		result.NotifyWatcherId = m.resources.Register(watch)
	} else {
		return result, statewatcher.EnsureErr(watch)
	}
	return result, nil
}

// ModelConfig returns the current model's configuration.
func (m *MongoModelWatcher) ModelConfig(ctx context.Context) (params.ModelConfigResult, error) {
	result := params.ModelConfigResult{}
	config, err := m.st.ModelConfig(ctx)
	if err != nil {
		return result, err
	}
	result.Config = config.AllAttrs()
	return result, nil
}
