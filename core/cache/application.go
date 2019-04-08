// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/pubsub"
)

const (
	// the application's charm url has changed.
	applicationCharmURLChange = "application-charmurl-change"
	// application config has changed.
	applicationConfigChange = "application-config-change"
)

func newApplication(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Application {
	a := &Application{
		metrics: metrics,
		hub:     hub,
	}
	a.Entity.removalDelta = a.removalDelta
	return a
}

// Application represents an application in a model.
type Application struct {
	Entity

	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details    ApplicationChange
	configHash string
	hashCache  *hashCache
}

// CharmURL returns the charm url string for this application.
func (a *Application) CharmURL() string {
	return a.details.CharmURL
}

// Config returns a copy of the current application config.
func (a *Application) Config() map[string]interface{} {
	a.mu.Lock()
	cfg := make(map[string]interface{}, len(a.details.Config))
	for k, v := range a.details.Config {
		cfg[k] = v
	}
	a.mu.Unlock()
	a.metrics.ApplicationConfigReads.Inc()
	return cfg
}

// WatchConfig creates a watcher for the application config.
func (a *Application) WatchConfig(keys ...string) *ConfigWatcher {
	w := newConfigWatcher(keys, a.hashCache, a.hub, a.topic(applicationConfigChange))
	a.registerWatcher(w)
	return w
}

func (a *Application) removalDelta() interface{} {
	return RemoveApplication{
		ModelUUID: a.details.ModelUUID,
		Name:      a.details.Name,
	}
}

// appCharmUrlChange contains an appName and it's charm URL.  To be used
// when publishing for applicationCharmURLChange.
type appCharmUrlChange struct {
	appName string
	chURL   string
}

func (a *Application) setDetails(details ApplicationChange) {
	a.mu.Lock()

	if a.details.CharmURL != details.CharmURL {
		a.hub.Publish(a.modelTopic(applicationCharmURLChange), appCharmUrlChange{appName: a.details.Name, chURL: details.CharmURL})
	}

	a.state = Active
	a.details = details
	hashCache, configHash := newHashCache(
		details.Config, a.metrics.ApplicationHashCacheHit, a.metrics.ApplicationHashCacheMiss)

	if configHash != a.configHash {
		a.configHash = configHash
		a.hashCache = hashCache
		a.hub.Publish(a.topic(applicationConfigChange), hashCache)
	}

	a.mu.Unlock()
}

// topic prefixes the input string with the model ID and application name.
// TODO (manadart 2019-03-14) The model ID will not be necessary when there is
// one hub per model.
func (a *Application) topic(suffix string) string {
	return a.details.ModelUUID + ":" + a.details.Name + ":" + suffix
}

func (a *Application) modelTopic(suffix string) string {
	return modelTopic(a.details.ModelUUID, suffix)
}
