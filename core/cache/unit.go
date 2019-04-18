// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

// Unit represents a unit in a cached model.
type Unit struct {
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

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

// Name returns the name of this unit.
func (u *Unit) Name() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.Name
}

// Application returns the application name of this unit.
func (u *Unit) Application() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.Application
}

// MachineId returns the ID of the machine hosting this unit.
func (u *Unit) MachineId() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.MachineId
}

// Subordinate returns a bool indicating whether this unit is a subordinate.
func (u *Unit) Subordinate() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.Subordinate
}

// Principal returns the name of the principal unit for the same application.
func (u *Unit) Principal() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.Principal
}

// CharmURL returns the charm URL for this unit's application.
func (u *Unit) CharmURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.details.CharmURL
}

func (u *Unit) setDetails(details UnitChange) {
	u.mu.Lock()

	machineChange := u.details.MachineId != details.MachineId
	u.details = details
	if machineChange {
		u.hub.Publish(u.modelTopic(modelUnitLXDProfileChange), u)
	}

	// TODO (manadart 2019-02-11): Maintain hash and publish changes.
	u.mu.Unlock()
}

func (u *Unit) modelTopic(suffix string) string {
	return modelTopic(u.details.ModelUUID, suffix)
}
