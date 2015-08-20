// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/persistence"
)

var logger = loggo.GetLogger("juju.workload.state")

// TODO(ericsnow) Add names.WorkloadTag and use it here?

// TODO(ericsnow) We need a worker to clean up dying workloads.

// The persistence methods needed for workloads in state.
type workloadsPersistence interface {
	Insert(info workload.Info) (bool, error)
	SetStatus(id string, status workload.CombinedStatus) (bool, error)
	List(ids ...string) ([]workload.Info, []string, error)
	ListAll() ([]workload.Info, error)
	Remove(id string) (bool, error)
}

// UnitWorkloads provides the functionality related to a unit's
// workloads, as needed by state.
type UnitWorkloads struct {
	// Persist is the persistence layer that will be used.
	Persist workloadsPersistence
	// Unit identifies the unit associated with the workloads.
	Unit names.UnitTag
	// Metadata is a function that returns the metadata for the unit's charm.
	Metadata func() (*charm.Meta, error)
}

// NewUnitWorkloads builds a UnitWorkloads for a unit.
func NewUnitWorkloads(st persistence.PersistenceBase, unit names.UnitTag, getMetadata func() (*charm.Meta, error)) *UnitWorkloads {
	persist := persistence.NewPersistence(st, unit)
	return &UnitWorkloads{
		Persist:  persist,
		Unit:     unit,
		Metadata: getMetadata,
	}
}

// Add registers the provided workload info in state.
func (ps UnitWorkloads) Track(info workload.Info) error {
	logger.Tracef("tracking %#v", info)
	if err := info.Validate(); err != nil {
		return errors.NewNotValid(err, "bad workload info")
	}

	ok, err := ps.Persist.Insert(info)
	if err != nil {
		return errors.Trace(err)
	}
	if !ok {
		return errors.NotValidf("workload %s (already in state)", info.ID())
	}

	return nil
}

// SetStatus updates the raw status for the identified workload to the
// provided value.
func (ps UnitWorkloads) SetStatus(id string, status workload.CombinedStatus) error {
	logger.Tracef("setting status for %q", id)
	if err := status.Validate(); err != nil {
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
// workload workloads associated with the unit. Missing workloads
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

// ListDefinitions builds the list of workload definitions found in the
// unit's charm metadata.
func (ps UnitWorkloads) Definitions() ([]charm.Workload, error) {
	meta, err := ps.Metadata()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var definitions []charm.Workload
	for _, definition := range meta.Workloads {
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// Remove removes the identified workload from state. It does not
// trigger the actual destruction of the workload.
func (ps UnitWorkloads) Untrack(id string) error {
	logger.Tracef("untracking %q", id)
	// If the record wasn't found then we're already done.
	_, err := ps.Persist.Remove(id)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
