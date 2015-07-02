// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/persistence"
)

// TODO(ericsnow) Add names.ProcessTag and use it here?

// TODO(ericsnow) We need a worker to clean up dying procs.

// TODO(ericsnow) Export ProcessesPersistence?

// The persistence methods needed for workload processes in state.
type processesPersistence interface {
	EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error)
	Insert(info process.Info) (bool, error)
	SetStatus(id string, status process.Status) (bool, error)
	List(ids ...string) ([]process.Info, []string, error)
	ListAll() ([]process.Info, error)
	Remove(id string) (bool, error)
}

// UnitProcesses provides the functionality related to a unit's
// processes, as needed by state.
type UnitProcesses struct {
	// Persist is the persistence layer that will be used.
	Persist processesPersistence
	// Unit identifies the unit associated with the processes.
	Unit names.UnitTag
}

// NewUnitProcesses builds a UnitProcesses for a charm/unit.
func NewUnitProcesses(st persistence.PersistenceBase, unit names.UnitTag, charm *names.CharmTag) *UnitProcesses {
	persist := persistence.NewPersistence(st, charm, &unit)
	return &UnitProcesses{
		Persist: persist,
		Unit:    unit,
	}
}

// Register adds the provided process info to state.
func (ps UnitProcesses) Register(info process.Info) error {
	if err := info.Validate(); err != nil {
		return errors.NewNotValid(err, "bad process info")
	}

	_, mismatched, err := ps.Persist.EnsureDefinitions(info.Process)
	if err != nil {
		return errors.Trace(err)
	}
	if len(mismatched) > 0 {
		return errors.NotValidf("mismatched definition for %q", info.Name)
	}

	ok, err := ps.Persist.Insert(info)
	if err != nil {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.Trace(err)
	}
	if !ok {
		// TODO(ericsnow) Remove the definition we may have just added?
		return errors.NotValidf("process %s (already in state)", info.ID())
	}

	return nil
}

// SetStatus updates the raw status for the identified process to the
// provided value.
func (ps UnitProcesses) SetStatus(id string, status process.Status) error {
	found, err := ps.Persist.SetStatus(id, status)
	if err != nil {
		return errors.Trace(err)
	}
	if !found {
		return errors.NotFoundf(id)
	}
	return nil
}

// List builds the list of process information for the provided process
// IDs. If none are provided then the list contains the info for all
// workload processes associated with the unit. Missing processes
// are ignored.
func (ps UnitProcesses) List(ids ...string) ([]process.Info, error) {
	if len(ids) == 0 {
		results, err := ps.Persist.ListAll()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return results, nil
	}

	results, _, err := ps.Persist.List(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO(ericsnow) Ensure that the number returned matches the
	// number expected.
	return results, nil
}

// Unregister removes the identified process from state. It does not
// trigger the actual destruction of the process.
func (ps UnitProcesses) Unregister(id string) error {
	// If the record wasn't found then we're already done.
	_, err := ps.Persist.Remove(id)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) Remove unit-based definition when no procs left.
	return nil
}
