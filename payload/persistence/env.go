// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// EnvPersistenceEntities provides all the information needed to produce
// a new EnvPersistence value.
type EnvPersistenceEntities interface {
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

// EnvPersistence provides the persistence functionality for the
// Juju environment as a whole.
type EnvPersistence struct {
	st EnvPersistenceEntities

	newUnitPersist func(name string) unitPersistence
}

// NewEnvPersistence wraps the "db" in a new EnvPersistence.
func NewEnvPersistence(db PersistenceBase, st EnvPersistenceEntities) *EnvPersistence {
	return &EnvPersistence{
		st: st,
		newUnitPersist: func(name string) unitPersistence {
			return NewPersistence(db, name)
		},
	}
}

// ListAll returns the list of all payloads in the environment.
func (ep *EnvPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	logger.Tracef("listing all payloads")

	machines, err := ep.st.Machines()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var payloads []payload.FullPayloadInfo
	for _, machine := range machines {
		units, err := ep.st.MachineUnits(machine)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, unit := range units {
			persist := ep.newUnitPersist(unit)

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
func listUnit(persist unitPersistence, unit, machine string) ([]payload.FullPayloadInfo, error) {
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
