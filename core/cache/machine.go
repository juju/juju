// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
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

// WatchContainers creates a PredicateStringsWatcher (strings watcher) to notify
// about added and removed containers on this machine.  The initial event
// contains a slice of the current container machine ids.
func (m *Machine) WatchContainers() (*PredicateStringsWatcher, error) {
	m.model.mu.Lock()

	// Create a compiled regexp to match containers on this machine.
	compiled, err := m.containerRegexp()
	if err != nil {
		return nil, err
	}

	// Gather initial slice of containers on this machine.
	machines := make([]string, 0)
	for k, v := range m.model.machines {
		if compiled.MatchString(v.details.Id) {
			machines = append(machines, k)
		}
	}

	w := newPredicateStringsWatcher(regexpPredicate(compiled), machines...)
	unsub := m.model.hub.Subscribe(m.modelTopic(modelAddRemoveMachine), w.changed)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	m.model.mu.Unlock()
	return w, nil
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
		appName := unit.details.Application
		unitName := unit.details.Name
		_, found := applications[appName]
		if found {
			applications[appName].units.Add(unitName)
			continue
		}
		app, foundApp := m.model.applications[appName]
		if !foundApp {
			// This is unlikely, but could happen.
			// If the unit has no machineId, it will be added
			// to what is watched when the machineId is assigned.
			// Otherwise return an error.
			if unit.details.MachineId != "" {
				return nil, errors.Errorf("programming error, unit %s has machineId but not application", unitName)
			}
			logger.Errorf("unit %s has no application, nor machine id, start watching when machine id assigned.", unitName)
			continue
		}
		info := appInfo{
			charmURL: app.details.CharmURL,
			units:    set.NewStrings(unitName),
		}
		ch, found := m.model.charms[app.details.CharmURL]
		if found {
			if !ch.details.LXDProfile.Empty() {
				info.charmProfile = &ch.details.LXDProfile
			}
		}
		applications[appName] = info
	}
	w := newMachineAppLXDProfileWatcher(
		m.model.topic(applicationCharmURLChange),
		m.model.topic(modelUnitLXDProfileChange),
		m.details.Id,
		applications,
		m.model,
		m.model.hub,
	)
	m.model.mu.Unlock()
	return w, nil
}

func (m *Machine) containerRegexp() (*regexp.Regexp, error) {
	regExp := fmt.Sprintf("^%s%s", m.details.Id, names.ContainerSnippet)
	return regexp.Compile(regExp)
}

func (m *Machine) modelTopic(suffix string) string {
	return modelTopic(m.details.ModelUUID, suffix)
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
