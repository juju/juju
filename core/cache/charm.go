// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"

	"github.com/juju/juju/core/lxdprofile"
)

func newCharm(metrics *ControllerGauges, hub *pubsub.SimpleHub, res *Resident) *Charm {
	c := &Charm{
		Resident: res,
		metrics:  metrics,
		hub:      hub,
	}
	return c
}

// Charm represents an charm in a model.
type Charm struct {
	// Resident identifies the charm as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

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
