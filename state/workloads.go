// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

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

type newUnitPayloadsFunc func(persist Persistence, unit string) (UnitPayloads, error)

var (
	newUnitPayloads newUnitPayloadsFunc
)

// SetWorkloadComponent registers the functions that provide the state
// functionality related to workloads.
func SetWorkloadsComponent(upFunc newUnitPayloadsFunc) {
	newUnitPayloads = upFunc
}

// UnitPayloads exposes interaction with workloads in state
// for a the given unit.
func (st *State) UnitPayloads(unit *Unit) (UnitPayloads, error) {
	if newUnitPayloads == nil {
		return nil, errors.Errorf("payloads not supported")
	}

	persist := st.newPersistence()
	unitWorkloads, err := newUnitPayloads(persist, unit.UnitTag().Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unitWorkloads, nil
}
