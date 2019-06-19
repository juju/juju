// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
)

// Unit represents a unit in a cached model.
type Unit struct {
	// Resident identifies the unit as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	model   *Model
	details UnitChange
}

func newUnit(model *Model, res *Resident) *Unit {
	return &Unit{
		Resident: res,
		model:    model,
	}
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// Name returns the name of this unit.
func (u *Unit) Name() string {
	return u.details.Name
}

// Application returns the application name of this unit.
func (u *Unit) Application() string {
	return u.details.Application
}

// MachineId returns the ID of the machine hosting this unit.
func (u *Unit) MachineId() string {
	return u.details.MachineId
}

// Subordinate returns a bool indicating whether this unit is a subordinate.
func (u *Unit) Subordinate() bool {
	return u.details.Subordinate
}

// Principal returns the name of the principal unit for the same application.
func (u *Unit) Principal() string {
	return u.details.Principal
}

// CharmURL returns the charm URL for this unit's application.
func (u *Unit) CharmURL() string {
	return u.details.CharmURL
}

// Ports returns the exposed ports for the unit.
func (u *Unit) Ports() []network.Port {
	return u.details.Ports
}

// WatchConfigSettings returns a new watcher that will notify when the
// effective application charm config for this unit changes.
func (u *Unit) WatchConfigSettings() (*CharmConfigWatcher, error) {
	cfg := charmConfigWatcherConfig{
		model:                u.model,
		unitName:             u.details.Name,
		appName:              u.details.Application,
		appConfigChangeTopic: fmt.Sprintf("%s:%s", u.details.Application, applicationConfigChange),
		branchChangeTopic:    branchChange,
		branchRemoveTopic:    modelBranchRemove,
		hub:                  u.model.hub,
		res:                  u.Resident,
	}

	w, err := newCharmConfigWatcher(cfg)
	return w, errors.Trace(err)
}

func (u *Unit) setDetails(details UnitChange) {
	// If this is the first receipt of details, set the removal message.
	if u.removalMessage == nil {
		u.removalMessage = RemoveUnit{
			ModelUUID: details.ModelUUID,
			Name:      details.Name,
		}
	}

	u.setStale(false)

	machineChange := u.details.MachineId != details.MachineId
	u.details = details
	if machineChange || u.details.Subordinate {
		u.model.hub.Publish(modelUnitAdd, u.copy())
	}
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (u *Unit) copy() Unit {
	cu := *u
	cu.details = cu.details.copy()
	return cu
}
