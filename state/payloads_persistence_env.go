// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// PayloadsEnvPersistenceBase provides all the information needed to produce
// a new PayloadsAllPersistence value.
type PayloadsEnvPersistenceBase interface {
	PayloadsPersistenceBase

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
	// ListAll returns all payloads associated with the unit.
	ListAll() ([]payload.Payload, error)
}

// PayloadsAllPersistence provides the persistence functionality for the
// Juju environment as a whole.
type PayloadsAllPersistence struct {
	base PayloadsEnvPersistenceBase

	newUnitPersist func(base PayloadsPersistenceBase, name string) unitPersistence
}

// NewPayloadsAllPersistence wraps the base in a new PayloadsAllPersistence.
func NewPayloadsAllPersistence(base PayloadsEnvPersistenceBase) *PayloadsAllPersistence {
	return &PayloadsAllPersistence{
		base:           base,
		newUnitPersist: newUnitPersistence,
	}
}

func newUnitPersistence(base PayloadsPersistenceBase, unit string) unitPersistence {
	return NewPayloadsPersistence(base, unit)
}

// ListAll returns the list of all payloads in the environment.
func (ep *PayloadsAllPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	machines, err := ep.base.Machines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []payload.FullPayloadInfo
	for _, machine := range machines {
		units, err := ep.base.MachineUnits(machine)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, unit := range units {
			persist := ep.newUnitPersist(ep.base, unit)

			unitPayloads, err := listUnitPayloads(persist, unit, machine)
			if err != nil {
				return nil, errors.Trace(err)
			}
			payloads = append(payloads, unitPayloads...)
		}
	}
	return payloads, nil
}

// listUnitPayloads returns all the payloads for the given unit.
func listUnitPayloads(persist unitPersistence, unit, machine string) ([]payload.FullPayloadInfo, error) {
	payloads, err := persist.ListAll()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fullPayloads []payload.FullPayloadInfo
	for _, pl := range payloads {
		fullPayloads = append(fullPayloads, payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		})
	}
	return fullPayloads, nil
}
