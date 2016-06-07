// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

// EnvPayloads exposes high-level interaction with all payloads
// in an model.
type EnvPayloads interface {
	// ListAll builds the list of registered payloads in the env and returns it.
	ListAll() ([]payload.FullPayloadInfo, error)
}

// UnitPayloads exposes high-level interaction with payloads for a unit.
type UnitPayloads interface {
	// Track tracks a payload in state. If a payload is
	// already being tracked for the same (unit, payload name, plugin ID)
	// then the request will fail. The unit must also be "alive".
	Track(payload payload.Payload) error
	// SetStatus sets the status of a payload. Only some fields
	// must be set on the provided info: Name, Status, Details.ID, and
	// Details.Status. If the payload is not in state then the request
	// will fail.
	SetStatus(docID, status string) error
	// List builds the list of payloads registered for
	// the given unit and IDs. If no IDs are provided then all
	// registered payloads for the unit are returned. In the case that
	// IDs are provided, any that are not in state are ignored and only
	// the found ones are returned. It is up to the caller to
	// extrapolate the list of missing IDs.
	List(ids ...string) ([]payload.Result, error)
	// LookUpReturns the Juju ID for the corresponding payload.
	LookUp(name, rawID string) (string, error)
	// Untrack removes the identified payload from state. If the
	// given ID is not in state then the request will fail.
	Untrack(id string) error
}

// TODO(ericsnow) Use a more generic component registration mechanism?

type newEnvPayloadsFunc func(Persistence) (EnvPayloads, error)
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

	db := st.newPersistence()
	envPayloads, err := newEnvPayloads(db)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return envPayloads, nil
}

// UnitPayloads exposes interaction with payloads in state
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
	unitPayloads, err := newUnitPayloads(persist, unitID, machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unitPayloads, nil
}
