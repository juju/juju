// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

const (
	applicationConfigChange = "application-config-change"
	applicationFieldChange  = "application-field-change"
)

func newApplication(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Application {
	m := &Application{
		metrics: metrics,
		hub:     hub,
	}
	return m
}

// Application represents an application in a model.
type Application struct {
	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    ApplicationChange
	configHash string
}

func (a *Application) WatchFields(comparitors ...func(*ApplicationDelta) bool) *ApplicationFieldWatcher {
	return newApplicationFieldWatcher(a.hub, a.topic(applicationFieldChange), comparitors)
}

// TODO (manadart 2018-03-13) Should we change this up and down the call stack
// so that pointers are passed instead of copies?
func (a *Application) setDetails(details ApplicationChange) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delta := &ApplicationDelta{old: a.details, new: details}
	a.details = details

	configHash, err := hash(details.Config)
	if err != nil {
		logger.Errorf("invariant error - application config should be yaml serializable and hashable, %v", err)
		configHash = ""
	}
	if configHash != a.configHash {
		a.configHash = configHash
		// TODO: publish config change...
	}

	a.hub.Publish(a.topic(applicationFieldChange), delta)
}

// topic prefixes the input string with the model ID and application name.
// TODO (manadart 2019-03-14) The model ID will not be necessary when there is
// one hub per model.
func (a *Application) topic(suffix string) string {
	return a.details.ModelUUID + ":" + a.details.Name + ":" + suffix
}
