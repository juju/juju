// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/persistence"
)

var logger = loggo.GetLogger("juju.workload.state")

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
	Unit string

	newID func() (string, error)
}

// NewUnitWorkloads builds a UnitWorkloads for a unit.
func NewUnitWorkloads(st persistence.PersistenceBase, unit string) *UnitWorkloads {
	persist := persistence.NewPersistence(st, unit)
	return &UnitWorkloads{
		Persist: persist,
		Unit:    unit,
		newID:   newID,
	}
}

func newID() (string, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "could not create new payload ID")
	}
	return uuid.String(), nil
}

// TODO(ericsnow) Return the new ID from Track()?

// Track inserts the provided workload info in state. The new Juju ID
// for the workload is returned.
func (uw UnitWorkloads) Track(info workload.Info) error {
	logger.Tracef("tracking %#v", info)
	if err := info.Validate(); err != nil {
		return errors.NewNotValid(err, "bad workload info")
	}

	id, err := uw.newID()
	if err != nil {
		return errors.Trace(err)
	}

	ok, err := uw.Persist.Track(id, info)
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
func (uw UnitWorkloads) SetStatus(id, status string) error {
	logger.Tracef("setting payload status for %q to %q", id, status)

	if err := workload.ValidateState(status); err != nil {
		return errors.Trace(err)
	}

	found, err := uw.Persist.SetStatus(id, status)
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
func (uw UnitWorkloads) List(ids ...string) ([]workload.Result, error) {
	logger.Tracef("listing %v", ids)
	var err error
	var workloads []workload.Info
	missingIDs := make(map[string]bool)
	if len(ids) == 0 {
		workloads, err = uw.Persist.ListAll()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _ = range workloads {
			ids = append(ids, "")
		}
	} else {
		var missing []string
		workloads, missing, err = uw.Persist.List(ids...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, id := range missing {
			missingIDs[id] = true
		}
	}

	var results []workload.Result
	i := 0
	for _, id := range ids {
		if missingIDs[id] {
			results = append(results, workload.Result{
				ID:       id,
				NotFound: true,
				Error:    errors.NotFoundf(id),
			})
			continue
		}
		wl := workloads[i]
		i += 1

		result := workload.Result{
			ID:       id,
			Workload: &wl,
		}
		if id == "" {
			// TODO(ericsnow) Do this more efficiently.
			id, err := uw.LookUp(wl.Name, wl.Details.ID)
			if err != nil {
				id = ""
				result.Error = errors.Trace(err)
			}
			result.ID = id
		}
		results = append(results, result)
	}
	return results, nil
}

// LookUp returns the payload ID for the given name/rawID pair.
func (uw UnitWorkloads) LookUp(name, rawID string) (string, error) {
	logger.Tracef("looking up payload id for %s/%s", name, rawID)

	id, err := uw.Persist.LookUp(name, rawID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// Untrack removes the identified workload from state. It does not
// trigger the actual destruction of the workload.
func (uw UnitWorkloads) Untrack(id string) error {
	logger.Tracef("untracking %q", id)
	// If the record wasn't found then we're already done.
	_, err := uw.Persist.Untrack(id)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
