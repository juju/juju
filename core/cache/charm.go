// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
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

	details CharmChange
}

// LXDProfile returns the lxd profile of this charm.
func (c *Charm) LXDProfile() lxdprofile.Profile {
	return c.details.LXDProfile
}

// DefaultConfig returns the default configuration settings for the charm.
func (c *Charm) DefaultConfig() map[string]interface{} {
	return c.details.DefaultConfig
}

func (c *Charm) setDetails(details CharmChange) {
	c.setRemovalMessage(RemoveCharm{
		ModelUUID: details.ModelUUID,
		CharmURL:  details.CharmURL,
	})

	c.details = details
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (c *Charm) copy() Charm {
	cc := *c
	cc.details = cc.details.copy()
	return cc
}
