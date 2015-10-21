// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

// EnvPayloads exposes high-level interaction with all workloads
// in an environment.
type EnvPayloads interface {
	// ListAll builds the list of registered payloads in the env and returns it.
	ListAll() ([]workload.FullPayloadInfo, error)
}

// TODO(ericsnow) Use a more generic component registration mechanism?

// PayloadsEnvPersistence provides all the information needed to produce
// a new EnvPayloads value.
type PayloadsEnvPersistence interface {
	Persistence

	// TODO(ericsnow) Drop the machine-related API and provide UnitTags()?

	// Machines builds the list of the names that identify
	// all machines in State.
	Machines() ([]string, error)

	// Machines builds the list of names that identify all units
	// for a given machine.
	MachineUnits(machineName string) ([]string, error)
}

type newEnvPayloadsFunc func(PayloadsEnvPersistence) (EnvPayloads, error)

var (
	newEnvPayloads newEnvPayloadsFunc
)

// TODO(ericsnow) Merge the 2 Set*Component funcs.

// SetPayloadComponent registers the functions that provide the state
// functionality related to payloads.
func SetPayloadsComponent(epFunc newEnvPayloadsFunc) {
	newEnvPayloads = epFunc
}

// EnvPayloads exposes interaction with payloads in state.
func (st *State) EnvPayloads() (EnvPayloads, error) {
	if newEnvPayloads == nil {
		return nil, errors.Errorf("payloads not supported")
	}

	persist := &payloadsEnvPersistence{
		Persistence: st.newPersistence(),
		st:          st,
	}
	envPayloads, err := newEnvPayloads(persist)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return envPayloads, nil
}

type payloadsEnvPersistence struct {
	Persistence
	st *State
}

// Machines implements PayloadsEnvPersistence.
func (ep *payloadsEnvPersistence) Machines() ([]string, error) {
	ms, err := ep.st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var names []string
	for _, m := range ms {
		names = append(names, m.Id())
	}
	return names, nil
}

// MachineUnits implements PayloadsEnvPersistence.
func (ep *payloadsEnvPersistence) MachineUnits(machine string) ([]string, error) {
	us, err := ep.st.UnitsFor(machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var names []string
	for _, u := range us {
		names = append(names, u.UnitTag().Id())
	}
	return names, nil
}
