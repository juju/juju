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

	"github.com/juju/juju/core/instance"
)

func newMachine(model *Model, res *resident) *Machine {
	m := &Machine{
		resident: res,
		model:    model,
	}
	return m
}

// Machine represents a machine in a model.
type Machine struct {
	// resident identifies the machine as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*resident

	model *Model
	mu    sync.Mutex

	modelUUID  string
	details    MachineChange
	configHash string
}

// Id returns the id string of this machine.
func (m *Machine) Id() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.details.Id
}

// InstanceId returns the provider specific instance id for this machine and
// returns not provisioned if instance id is empty
func (m *Machine) InstanceId() (instance.Id, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.details.InstanceId == "" {
		return "", errors.NotProvisionedf("machine %v", m.details.Id)
	}
	return instance.Id(m.details.InstanceId), nil
}

// CharmProfiles returns the cached list of charm profiles for the machine
func (m *Machine) CharmProfiles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.details.CharmProfiles
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
	// Create a compiled regexp to match containers on this machine.
	compiled, err := m.containerRegexp()
	if err != nil {
		return nil, err
	}

	// Gather initial slice of containers on this machine.
	machines := make([]string, 0)
	for k, v := range m.model.Machines() {
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

	m.registerWorker(w)
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

	m.mu.Lock()
	defer m.mu.Unlock()

	applications := make(map[string]appInfo)
	for _, unit := range units {
		appName := unit.Application()
		unitName := unit.Name()
		_, found := applications[appName]
		if found {
			applications[appName].units.Add(unitName)
			continue
		}

		app, err := m.model.Application(appName)
		if errors.IsNotFound(err) {
			// This is unlikely, but could happen.
			// If the unit has no machineId, it will be added
			// to what is watched when the machineId is assigned.
			// Otherwise return an error.
			if unit.MachineId() != "" {
				return nil, errors.Errorf("programming error, unit %s has machineId but not application", unitName)
			}
			logger.Errorf("unit %s has no application, nor machine id, start watching when machine id assigned.", unitName)
			m.model.metrics.LXDProfileChangeError.Inc()
			continue
		}

		chURL := app.CharmURL()
		info := appInfo{
			charmURL: chURL,
			units:    set.NewStrings(unitName),
		}

		ch, found := m.model.charms[chURL]
		if found {
			lxdProfile := ch.LXDProfile()
			if !lxdProfile.Empty() {
				info.charmProfile = lxdProfile
			}
		}
		applications[appName] = info
	}

	w := newMachineAppLXDProfileWatcher(MachineAppLXDProfileConfig{
		appTopic:     m.model.topic(applicationCharmURLChange),
		unitTopic:    m.model.topic(modelUnitLXDProfileChange),
		machineId:    m.details.Id,
		applications: applications,
		modeler:      m.model,
		metrics:      m.model.metrics,
		hub:          m.model.hub,
		resident:     m.resident,
	})
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
