// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/persistence"
)

var logger = loggo.GetLogger("juju.workload.state")

// TODO(ericsnow) Add names.WorkloadTag and use it here?

// TODO(ericsnow) We need a worker to clean up dying workloads.

// The persistence methods needed for workloads in state.
type workloadsPersistence interface {
	Track(id string, info workload.Info) (bool, error)
	// SetStatus updates the status for a payload.
	SetStatus(id, status string) (bool, error)
	List(ids ...string) ([]workload.Info, []string, error)
	ListAll() ([]workload.Info, error)
	LookUp(name, rawID string) (string, error)
	Untrack(id string) (bool, error)
}

// UnitWorkloads provides the functionality related to a unit's
// workloads, as needed by state.
type UnitWorkloads struct {
	// Persist is the persistence layer that will be used.
	Persist workloadsPersistence
	// Unit identifies the unit associated with the workloads.
	Unit names.UnitTag

	NewID func(workload.Info) (string, error)
}

// NewUnitWorkloads builds a UnitWorkloads for a unit.
func NewUnitWorkloads(st persistence.PersistenceBase, unit names.UnitTag) *UnitWorkloads {
	persist := persistence.NewPersistence(st, unit)
	return &UnitWorkloads{
		Persist: persist,
		Unit:    unit,
		NewID:   newID,
	}
}

func newID(workload.Info) (string, error) {
	return workload.NewID()
}

// TODO(ericsnow) Return the new ID from Track()?

// Track inserts the provided workload info in state. The new Juju ID
// for the workload is returned.
func (ps UnitWorkloads) Track(info workload.Info) error {
	logger.Tracef("tracking %#v", info)
	if err := info.Validate(); err != nil {
		return errors.NewNotValid(err, "bad workload info")
	}

	id, err := ps.NewID(info)
	if err != nil {
		return errors.Trace(err)
	}

	ok, err := ps.Persist.Track(id, info)
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return errors.NotValidf("workload %s (already in state)", id)
	}

	return nil
}

// SetStatus updates the raw status for the identified workload to the
// provided value.
func (ps UnitWorkloads) SetStatus(id, status string) error {
	logger.Tracef("setting payload status for %q to %q", id, status)

	if err := workload.ValidateState(status); err != nil {
		return errors.Trace(err)
	}

	found, err := ps.Persist.SetStatus(id, status)
	if err != nil {
		return errors.Trace(err)
	}
	if !found {
		return errors.NotFoundf(id)
	}
	return nil
}

// List builds the list of workload information for the provided workload
// IDs. If none are provided then the list contains the info for all
// workloads associated with the unit. Missing workloads
// are ignored.
func (ps UnitWorkloads) List(ids ...string) ([]workload.Info, error) {
	logger.Tracef("listing %v", ids)
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
	return results, nil
}

// LookUp returns the payload ID for the given name/rawID pair.
func (ps UnitWorkloads) LookUp(name, rawID string) (string, error) {
	logger.Tracef("looking up payload id for %s/%s", name, rawID)

	id, err := ps.Persist.LookUp(name, rawID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// Untrack removes the identified workload from state. It does not
// trigger the actual destruction of the workload.
func (ps UnitWorkloads) Untrack(id string) error {
	logger.Tracef("untracking %q", id)
	// If the record wasn't found then we're already done.
	_, err := ps.Persist.Untrack(id)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
