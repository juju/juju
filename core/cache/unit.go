// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/settings"
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

// Life returns the current life of the unit.
func (u *Unit) Life() life.Value {
	return u.details.Life
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

// Config settings returns the effective charm configuration for this unit
// taking into account whether it is tracking a model branch.
func (u *Unit) ConfigSettings() (charm.Settings, error) {
	if u.details.CharmURL == "" {
		return nil, errors.New("unit's charm URL must be set before retrieving config")
	}

	appName := u.details.Application
	app, err := u.model.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg := app.Config()
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	// Apply any branch-based deltas to the master settings.
	var deltas settings.ItemChanges
	branches := u.model.Branches()
	for _, b := range branches {
		if units := b.AssignedUnits()[appName]; len(units) > 0 {
			if set.NewStrings(units...).Contains(u.details.Name) {
				deltas = b.AppConfig(appName)
				break
			}
		}
	}

	for _, delta := range deltas {
		switch {
		case delta.IsAddition(), delta.IsModification():
			cfg[delta.Key] = delta.NewValue
		case delta.IsDeletion():
			delete(cfg, delta.Key)
		}
	}

	// Fill in any empty values with charm defaults.
	ch, err := u.model.Charm(u.details.CharmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmDefaults := ch.DefaultConfig()

	for k, v := range charmDefaults {
		if _, ok := cfg[k]; !ok {
			cfg[k] = v
		}
	}

	return cfg, nil
}

// WatchConfigSettings returns a new watcher that will notify when the
// effective application charm config for this unit changes.
func (u *Unit) WatchConfigSettings() (*CharmConfigWatcher, error) {
	if u.details.CharmURL == "" {
		return nil, errors.New("unit's charm URL must be set before watching config")
	}

	cfg := charmConfigWatcherConfig{
		model:                u.model,
		unitName:             u.details.Name,
		appName:              u.details.Application,
		charmURL:             u.details.CharmURL,
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
	var newSubordinate bool

	// If this is the first receipt of details, set the removal message.
	if u.removalMessage == nil {
		u.removalMessage = RemoveUnit{
			ModelUUID: details.ModelUUID,
			Name:      details.Name,
		}

		newSubordinate = details.Subordinate
	}

	u.setStale(false)

	landingOnMachine := u.details.MachineId != details.MachineId
	u.details = details
	toPublish := u.copy()

	// Publish a unit addition event if a unit gets a machine ID for the first
	// time, or if this is a subordinate that was not previously in the cache.
	if landingOnMachine || newSubordinate {
		u.model.hub.Publish(modelUnitAdd, toPublish)
	}

	// Publish change event for those that may be waiting.
	u.model.hub.Publish(unitChangeTopic(details.Name), &toPublish)
}

// copy returns a copy of the unit, ensuring appropriate deep copying.
func (u *Unit) copy() Unit {
	cu := *u
	cu.details = cu.details.copy()
	return cu
}
