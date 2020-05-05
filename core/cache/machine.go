// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
)

const (
	machineProvisioned = "machine-provisioned"
)

func newMachine(model *Model, res *Resident) *Machine {
	m := &Machine{
		Resident: res,
		model:    model,
	}
	return m
}

// Machine represents a machine in a model.
type Machine struct {
	// Resident identifies the machine as a type-agnostic cached entity
	// and tracks resources that it is responsible for cleaning up.
	*Resident

	model *Model

	details    MachineChange
	configHash string
}

// Note that these property accessors are not lock-protected.
// They are intended for calling from external packages that have retrieved a
// deep copy from the cache.

// Id returns the id string of this machine.
func (m *Machine) Id() string {
	return m.details.Id
}

// InstanceId returns the provider specific instance id for this machine and
// returns not provisioned if instance ID is empty.
func (m *Machine) InstanceId() (instance.Id, error) {
	if m.details.InstanceId == "" {
		return "", errors.NotProvisionedf("machine %v", m.details.Id)
	}
	return instance.Id(m.details.InstanceId), nil
}

// CharmProfiles returns the cached list of charm profiles for the machine.
func (m *Machine) CharmProfiles() []string {
	return m.details.CharmProfiles
}

// ContainerType returns the cached container type hosting this machine.
func (m *Machine) ContainerType() instance.ContainerType {
	return instance.ContainerType(m.details.ContainerType)
}

// Config returns configuration settings for the machine.
func (m *Machine) Config() map[string]interface{} {
	return m.details.Config
}

// Units returns all the units that have been assigned to the machine
// including subordinates.
func (m *Machine) Units() ([]Unit, error) {
	units := m.model.Units()

	var result []Unit
	for unitName, unit := range units {
		if unit.MachineId() == m.details.Id {
			result = append(result, unit)
		}
		if unit.details.Subordinate {
			principalUnit, found := units[unit.Principal()]
			if !found {
				return result, errors.NotFoundf("principal unit %q for subordinate %s", unit.Principal(), unitName)
			}
			if principalUnit.MachineId() == m.details.Id {
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
	deregister := m.registerWorker(w)
	unsub := m.model.hub.Subscribe(modelAddRemoveMachine, w.changed)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		deregister()
		return nil
	})

	m.registerWorker(w)
	return w, nil
}

// WatchLXDProfileVerificationNeeded notifies if any of the following happen
// relative to this machine:
//     1. A new unit whose charm has an LXD profile is added.
//     2. A unit being removed has a profile and other units
//        exist on the machine.
//     3. The LXD profile of an application with a unit on this
//        machine is added, removed, or exists.
//     4. The machine's instanceId is changed, indicating it
//        has been provisioned.
func (m *Machine) WatchLXDProfileVerificationNeeded() (*MachineLXDProfileWatcher, error) {
	return newMachineLXDProfileWatcher(MachineLXDProfileWatcherConfig{
		appTopic:         applicationCharmURLChange,
		provisionedTopic: m.topic(machineProvisioned),
		unitAddTopic:     modelUnitAdd,
		unitRemoveTopic:  modelUnitRemove,
		machine:          m.copy(),
		modeler:          m.model,
		metrics:          m.model.metrics,
		hub:              m.model.hub,
		resident:         m.Resident,
	})
}

func (m *Machine) containerRegexp() (*regexp.Regexp, error) {
	regExp := fmt.Sprintf("^%s%s", m.details.Id, names.ContainerSnippet)
	return regexp.Compile(regExp)
}

func (m *Machine) setDetails(details MachineChange) {
	// If this is the first receipt of details, set the removal message.
	if m.removalMessage == nil {
		m.removalMessage = RemoveMachine{
			ModelUUID: details.ModelUUID,
			Id:        details.Id,
		}
	}

	m.setStale(false)

	provisioned := details.InstanceId != m.details.InstanceId
	m.details = details

	if provisioned {
		m.model.hub.Publish(m.topic(machineProvisioned), nil)
	}

	configHash, err := hashSettings(details.Config)
	if err != nil {
		logger.Errorf("invariant error - machine config should be yaml serializable and hashable, %v", err)
		configHash = ""
	}
	if configHash != m.configHash {
		m.configHash = configHash
		// TODO: publish config change...
	}
}

func (m *Machine) copy() Machine {
	cm := *m
	cm.details = cm.details.copy()
	return cm
}

func (m *Machine) topic(suffix string) string {
	return m.details.Id + ":" + suffix
}
