// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/pubsub"
)

// Unit represents an unit in a cached model.
type Unit struct {
	Entity

	metrics *ControllerGauges
	hub     *pubsub.SimpleHub

	details    UnitChange
	configHash string
}

func newUnit(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Unit {
	u := &Unit{
		metrics: metrics,
		hub:     hub,
	}
	return u
}

// Application returns the application name of this unit.
func (u *Unit) Application() string {
	return u.details.Application
}

func (u *Unit) setDetails(details UnitChange) {
	u.mu.Lock()
	if u.details.MachineId != details.MachineId {
		u.hub.Publish(u.modelTopic(modelUnitLXDProfileChange), u)
	}
	u.state = Active
	u.details = details

	// TODO (manadart 2019-02-11): Maintain hash and publish changes.
	u.mu.Unlock()
}

func (u *Unit) modelTopic(suffix string) string {
	return modelTopic(u.details.ModelUUID, suffix)
}
