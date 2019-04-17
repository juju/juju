// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"

	"github.com/juju/juju/core/lxdprofile"
)

func newCharm(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Charm {
	c := &Charm{
		metrics: metrics,
		hub:     hub,
	}
	return c
}

// Charm represents an charm in a model.
type Charm struct {
	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details CharmChange
}

// LXDProfile returns the lxd profile of this charm.
func (c *Charm) LXDProfile() lxdprofile.Profile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.details.LXDProfile
}

func (c *Charm) setDetails(details CharmChange) {
	c.mu.Lock()
	c.details = details
	c.mu.Unlock()
}
