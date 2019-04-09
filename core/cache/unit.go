// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/pubsub"
)

// Unit represents an unit in a cached model.
type Unit struct {
	entity

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
	// wire up the removalDelta so that the entity can collate all the deltas
	// during a sweep phase. If this isn't correctly wired up, an error will be
	// returned during the sweeping phase.
	u.entity.removalDelta = u.removalDelta
	return u
}

// Application returns the application name of this unit.
func (u *Unit) Application() string {
	return u.details.Application
}

// removalDelta returns a delta that is required to remove the Unit. If this
// is not correctly wired up when setting up the Unit, then a error will be
// returned stating this fact when the Sweep phase of the GC.
func (u *Unit) removalDelta() interface{} {
	return RemoveUnit{
		ModelUUID: u.details.ModelUUID,
		Name:      u.details.Name,
	}
}

// remove cleans up any associated data with the unit
func (u *Unit) remove() {
	// TODO (stickupkid): clean watchers
}

func (u *Unit) setDetails(details UnitChange) {
	u.mu.Lock()
	if u.details.MachineId != details.MachineId {
		u.hub.Publish(u.modelTopic(modelUnitLXDProfileChange), u)
	}
	u.freshness = fresh
	u.details = details

	// TODO (manadart 2019-02-11): Maintain hash and publish changes.
	u.mu.Unlock()
}

func (u *Unit) modelTopic(suffix string) string {
	return modelTopic(u.details.ModelUUID, suffix)
}
