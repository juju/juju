// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

func newCharm(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Charm {
	c := &Charm{
		metrics: metrics,
		hub:     hub,
	}
	return c
}

// Application represents an application in a model.
type Charm struct {
	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details CharmChange
}

func (c *Charm) setDetails(details CharmChange) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.details = details
}
