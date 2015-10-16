// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

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

	// MachineNames builds the list of the names that identify
	// all machines in State.
	MachineNames() ([]string, error)

	// MachineUnits builds the list of tags that identify all units
	// for a given machine.
	MachineUnits(machineName string) ([]names.UnitTag, error)
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

// MachineNames implements PayloadsEnvPersistence.
func (ep *payloadsEnvPersistence) MachineNames() ([]string, error) {
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
func (ep *payloadsEnvPersistence) MachineUnits(machine string) ([]names.UnitTag, error) {
	us, err := ep.st.UnitsFor(machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var tags []names.UnitTag
	for _, u := range us {
		tags = append(tags, u.UnitTag())
	}
	return tags, nil
}
