// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
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

func (m *Application) setDetails(details ApplicationChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.details = details

	configHash, err := hash(details.Config)
	if err != nil {
		logger.Errorf("invariant error - application config should be yaml serializable and hashable, %v", err)
		configHash = ""
	}
	if configHash != m.configHash {
		m.configHash = configHash
		// TODO: publish config change...
	}
}
