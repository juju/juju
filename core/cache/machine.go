// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/juju/core/instance"
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

	details    MachineChange
	configHash string
}

// Id returns the id string of this machine.
func (m *Machine) Id() string {
	return m.details.Id
}

// InstanceId returns the provider specific instance id for this machine and
// returns not provisioned if instance id is empty
func (m *Machine) InstanceId() (instance.Id, error) {
	if m.details.InstanceId == "" && m.details.HardwareCharacteristics == nil {
		return "", errors.NotProvisionedf("machine %v", m.Id())
	}
	return instance.Id(m.details.InstanceId), nil
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
				return nil, errors.NotFoundf("principal unit %q for subordinate %s", unit.details.Principal, unitName)
			}
			if principalUnit.details.MachineId == m.details.Id {
				result = append(result, unit)
			}
		}
	}
	return result, nil
}

type MachineAppLXDProfileWatcher struct {
	*notifyWatcherBase
}

func (m *Machine) WatchApplicationLXDProfiles() *MachineAppLXDProfileWatcher {
	return nil
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
