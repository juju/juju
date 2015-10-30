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

// UnitPayloads exposes high-level interaction with workloads for a unit.
type UnitPayloads interface {
	// Track tracks a workload in state. If a workload is
	// already being tracked for the same (unit, workload name, plugin ID)
	// then the request will fail. The unit must also be "alive".
	Track(payload workload.Payload) error
	// SetStatus sets the status of a workload. Only some fields
	// must be set on the provided info: Name, Status, Details.ID, and
	// Details.Status. If the workload is not in state then the request
	// will fail.
	SetStatus(docID, status string) error
	// List builds the list of workloads registered for
	// the given unit and IDs. If no IDs are provided then all
	// registered workloads for the unit are returned. In the case that
	// IDs are provided, any that are not in state are ignored and only
	// the found ones are returned. It is up to the caller to
	// extrapolate the list of missing IDs.
	List(ids ...string) ([]workload.Result, error)
	// LookUpReturns the Juju ID for the corresponding workload.
	LookUp(name, rawID string) (string, error)
	// Untrack removes the identified workload from state. If the
	// given ID is not in state then the request will fail.
	Untrack(id string) error
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
type newUnitPayloadsFunc func(persist Persistence, unit, machine string) (UnitPayloads, error)

// TODO(ericsnow) Merge the 2 vars
var (
	newEnvPayloads  newEnvPayloadsFunc
	newUnitPayloads newUnitPayloadsFunc
)

// SetPayloadComponent registers the functions that provide the state
// functionality related to payloads.
func SetPayloadsComponent(epFunc newEnvPayloadsFunc, upFunc newUnitPayloadsFunc) {
	newEnvPayloads = epFunc
	newUnitPayloads = upFunc
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

// UnitPayloads exposes interaction with workloads in state
// for a the given unit.
func (st *State) UnitPayloads(unit *Unit) (UnitPayloads, error) {
	if newUnitPayloads == nil {
		return nil, errors.Errorf("payloads not supported")
	}

	machineID, err := unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitID := unit.UnitTag().Id()

	persist := st.newPersistence()
	unitWorkloads, err := newUnitPayloads(persist, unitID, machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unitWorkloads, nil
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
