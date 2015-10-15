// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/workload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

// UnitWorkloads exposes high-level interaction with workloads for a unit.
type UnitWorkloads interface {
	// Track tracks a workload in state. If a workload is
	// already being tracked for the same (unit, workload name, plugin ID)
	// then the request will fail. The unit must also be "alive".
	Track(info workload.Info) error
	// SetStatus sets the status of a workload. Only some fields
	// must be set on the provided info: Name, Status, Details.ID, and
	// Details.Status. If the workload is not in state then the request
	// will fail.
	SetStatus(class, id, status string) error
	// List builds the list of workloads registered for
	// the given unit and IDs. If no IDs are provided then all
	// registered workloads for the unit are returned. In the case that
	// IDs are provided, any that are not in state are ignored and only
	// the found ones are returned. It is up to the caller to
	// extrapolate the list of missing IDs.
	List(ids ...string) ([]workload.Info, error)
	// Untrack removes the identified workload from state. If the
	// given ID is not in state then the request will fail.
	Untrack(id string) error
}

// TODO(ericsnow) Use a more generic component registration mechanism?

type newUnitWorkloadsFunc func(persist Persistence, unit names.UnitTag) (UnitWorkloads, error)

var (
	newUnitWorkloads newUnitWorkloadsFunc
)

// SetWorkloadComponent registers the functions that provide the state
// functionality related to workloads.
func SetWorkloadsComponent(upFunc newUnitWorkloadsFunc) {
	newUnitWorkloads = upFunc
}

// UnitWorkloads exposes interaction with workloads in state
// for a the given unit.
func (st *State) UnitWorkloads(unit *Unit) (UnitWorkloads, error) {
	if newUnitWorkloads == nil {
		return nil, errors.Errorf("workloads not supported")
	}

	persist := st.newPersistence()
	unitWorkloads, err := newUnitWorkloads(persist, unit.UnitTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unitWorkloads, nil
}
