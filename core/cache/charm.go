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
	// wire up the removalDelta so that the entity can collate all the deltas
	// during a sweep phase. If this isn't correctly wired up, an error will be
	// returned during the sweeping phase.
	c.entity.removalDelta = c.removalDelta
	return c
}

// Charm represents an charm in a model.
type Charm struct {
	entity

	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details CharmChange
}

// LXDProfile returns the lxd profile of this charm.
func (c *Charm) LXDProfile() lxdprofile.Profile {
	return c.details.LXDProfile
}

// removalDelta returns a delta that is required to remove the Charm. If this
// is not correctly wired up when setting up the Charm, then a error will be
// returned stating this fact when the Sweep phase of the GC.
func (c *Charm) removalDelta() interface{} {
	return RemoveCharm{
		ModelUUID: c.details.ModelUUID,
		CharmURL:  c.details.CharmURL,
	}
}

func (c *Charm) setDetails(details CharmChange) {
	c.mu.Lock()
	c.freshness = fresh
	c.details = details
	c.mu.Unlock()
}
