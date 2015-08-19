// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

// UnitWorkloads exposes high-level interaction with workloads for a unit.
type UnitWorkloads interface {
	// Add registers a workload in state. If a workload is
	// already registered for the same (unit, workload name, plugin ID)
	// then the request will fail. The unit must also be "alive".
	Add(info workload.Info) error
	// SetStatus sets the status of a workload. Only some fields
	// must be set on the provided info: Name, Status, Details.ID, and
	// Details.Status. If the workload is not in state then the request
	// will fail.
	SetStatus(id string, status workload.CombinedStatus) error
	// List builds the list of workloads registered for
	// the given unit and IDs. If no IDs are provided then all
	// registered workloads for the unit are returned. In the case that
	// IDs are provided, any that are not in state are ignored and only
	// the found ones are returned. It is up to the caller to
	// extrapolate the list of missing IDs.
	List(ids ...string) ([]workload.Info, error)
	// ListDefinitions builds the list of workload definitions found
	// in the unit's charm metadata.
	ListDefinitions() ([]charm.Process, error)
	// Remove removes the identified workload from state. If the
	// given ID is not in state then the request will fail.
	Remove(id string) error
}

// TODO(ericsnow) Use a more generic component registration mechanism?

type newUnitWorkloadsFunc func(persist Persistence, unit names.UnitTag, getMetadata func() (*charm.Meta, error)) (UnitWorkloads, error)

var (
	newUnitWorkloads newUnitWorkloadsFunc
)

// SetWorkloadComponent registers the functions that provide the state
// functionality related to workloads.
func SetWorkloadsComponent(upFunc newUnitWorkloadsFunc) {
	newUnitWorkloads = upFunc
}

// UnitProcesses exposes interaction with workload processes in state
// for a the given unit.
func (st *State) UnitProcesses(unit *Unit) (UnitProcesses, error) {
	if newUnitProcesses == nil {
		return nil, errors.Errorf("workload processes not supported")
	}

	getMetadata := func() (*charm.Meta, error) {
		// TODO(ericsnow) Freeze the metadata at this moment?
		// TODO(ericsnow) Worry about refreshing the unit?
		ch, err := unit.charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ch.Meta(), nil
	}

	persist := st.newPersistence()
	unitProcs, err := newUnitProcesses(persist, unit.UnitTag(), getMetadata)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return unitProcs, nil
}
