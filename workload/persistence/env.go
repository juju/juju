// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// EnvPersistenceBase provides all the information needed to produce
// a new EnvPersistence value.
type EnvPersistenceBase interface {
	PersistenceBase

	// Machines builds the list of the names that identify
	// all machines in State.
	Machines() ([]string, error)

	// MachineUnits builds the list of names that identify all units
	// for a given machine.
	MachineUnits(machineName string) ([]string, error)
}

// unitPersistence describes the per-unit functionality needed
// for env persistence.
type unitPersistence interface {
	// ListAll returns all workloads associated with the unit.
	ListAll() ([]workload.Payload, error)
}

// EnvPersistence provides the persistence functionality for the
// Juju environment as a whole.
type EnvPersistence struct {
	base EnvPersistenceBase

	newUnitPersist func(base PersistenceBase, name string) unitPersistence
}

// NewEnvPersistence wraps the base in a new EnvPersistence.
func NewEnvPersistence(base EnvPersistenceBase) *EnvPersistence {
	return &EnvPersistence{
		base:           base,
		newUnitPersist: newUnitPersistence,
	}
}

func newUnitPersistence(base PersistenceBase, unit string) unitPersistence {
	return NewPersistence(base, unit)
}

// ListAll returns the list of all payloads in the environment.
func (ep *EnvPersistence) ListAll() ([]workload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	machines, err := ep.base.Machines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []workload.FullPayloadInfo
	for _, machine := range machines {
		units, err := ep.base.MachineUnits(machine)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, unit := range units {
			persist := ep.newUnitPersist(ep.base, unit)

			unitPayloads, err := listUnit(persist, unit, machine)
			if err != nil {
				return nil, errors.Trace(err)
			}
			payloads = append(payloads, unitPayloads...)
		}
	}
	return payloads, nil
}

// listUnit returns all the payloads for the given unit.
func listUnit(persist unitPersistence, unit, machine string) ([]workload.FullPayloadInfo, error) {
	payloads, err := persist.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fullPayloads []workload.FullPayloadInfo
	for _, pl := range payloads {
		fullPayloads = append(fullPayloads, workload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		})
	}
	return fullPayloads, nil
}
