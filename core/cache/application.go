// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

const (
	applicationConfigChange = "application-config-change"
)

func newApplication(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Application {
	a := &Application{
		metrics: metrics,
		hub:     hub,
	}
	return a
}

// Application represents an application in a model.
type Application struct {
	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    ApplicationChange
	configHash string
	hashCache  *hashCache
}

// Config returns the current application config.
func (a *Application) Config() map[string]interface{} {
	a.mu.Lock()
	a.metrics.ModelConfigReads.Inc()
	a.mu.Unlock()
	return a.details.Config
}

// WatchConfig creates a watcher for the application config.
func (a *Application) WatchConfig(keys ...string) *ConfigWatcher {
	return newConfigWatcher(keys, a.hashCache, a.hub, a.topic(applicationConfigChange))
}

func (a *Application) setDetails(details ApplicationChange) {
	a.mu.Lock()

	a.details = details
	hashCache, configHash := newHashCache(details.Config, nil, nil)
	if configHash != a.configHash {
		a.configHash = configHash
		a.hashCache = hashCache
		a.hub.Publish(a.topic(applicationConfigChange), hashCache)
	}

	defer a.mu.Unlock()
}

// topic prefixes the input string with the model ID and application name.
// TODO (manadart 2019-03-14) The model ID will not be necessary when there is
// one hub per model.
func (a *Application) topic(suffix string) string {
	return a.details.ModelUUID + ":" + a.details.Name + ":" + suffix
}
