// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

func newMachine(model *Model) *Machine {
	m := &Machine{
		model: model,
	}
	return m
}

// Machine represents a machine in a model.
type Machine struct {
	model *Model
	mu    sync.Mutex

	modelUUID  string
	details    MachineChange
	configHash string
}

// Id returns the id string of this machine.
func (m *Machine) Id() string {
	return m.details.Id
}

// Units returns all the units that have been assigned to the machine
// including subordinates.
func (m *Machine) Units() ([]*Unit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*Unit, 0)
	for unitName, unit := range m.model.units {
		if unit.details.MachineId == m.details.Id {
			result = append(result, unit)
		}
		if unit.details.Subordinate {
			principalUnit, found := m.model.units[unit.details.Principal]
			if !found {
				return result, errors.NotFoundf("principal unit %q for subordinate %s", unit.details.Principal, unitName)
			}
			if principalUnit.details.MachineId == m.details.Id {
				result = append(result, unit)
			}
		}
	}
	return result, nil
}

func (m *Machine) setDetails(details MachineChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.details = details

	configHash, err := hash(details.Config)
	if err != nil {
		logger.Errorf("invariant error - machine config should be yaml serializable and hashable, %v", err)
		configHash = ""
	}
	if configHash != m.configHash {
		m.configHash = configHash
		// TODO: publish config change...
	}
}

// WatchApplicationLXDProfiles notifies if any of the following happen
// relative to this machine:
//     1. A new unit whose charm has an lxd profile is added.
//     2. A unit being removed has a profile and other units
//        exist on the machine.
//     3. The lxdprofile of an application with a unit on this
//        machine is added, removed, or exists.
func (m *Machine) WatchApplicationLXDProfiles() (*MachineAppLXDProfileWatcher, error) {
	units, err := m.Units()
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get units to start MachineAppLXDProfileWatcher")
	}
	m.model.mu.Lock()
	applications := make(map[string]appInfo)
	for _, unit := range units {
		_, found := applications[unit.details.Application]
		if found {
			applications[unit.details.Application].units.Add(unit.details.Name)
			continue
		}
		app, foundApp := m.model.applications[unit.details.Application]
		if !foundApp {
			// This is unlikely, but could happen.
			// If the unit has no machineId, it will be added
			// to what is watched when the machineId is assigned.
			// Otherwise return an error.
			if unit.details.MachineId != "" {
				return nil, errors.Errorf("programming error, unit %s has machineId but not application", unit.details.Name)
			}
			logger.Errorf("unit %s has no application, nor machine id, start watching when machine id assigned.", unit.details.Name)
			continue
		}
		info := appInfo{
			charmURL: app.details.CharmURL,
			units:    set.NewStrings(unit.details.Name),
		}
		ch, found := m.model.charms[app.details.CharmURL]
		if found {
			if !ch.details.LXDProfile.Empty() {
				info.charmProfile = &ch.details.LXDProfile
			}
		}
		applications[unit.details.Application] = info
	}
	w := newMachineAppLXDProfileWatcher(
		m.model.topic(applicationCharmURLChange),
		m.model.topic(modelUnitLXDProfileChange),
		m.details.Id,
		applications,
		m.model.Application,
		m.model.Charm,
		m.model.Unit,
		m.model.hub,
	)
	m.model.mu.Unlock()
	return w, nil
}
