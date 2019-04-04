// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
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
	Entity

	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details CharmChange
}

// LXDProfile returns the lxd profile of this charm.
func (c *Charm) LXDProfile() lxdprofile.Profile {
	return c.details.LXDProfile
}

func (c *Charm) RemovalDelta() interface{} {
	return RemoveCharm{
		ModelUUID: c.details.ModelUUID,
		CharmURL:  c.details.CharmURL,
	}
}

func (c *Charm) setDetails(details CharmChange) {
	c.mu.Lock()
	c.state = Active
	c.details = details
	c.mu.Unlock()
}
