// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/internal"
)

// UnitWorkloads exposes the State functionality for a unit's workloads.
type UnitWorkloads interface {
	// Track tracks a workload for the unit and info.
	Track(info workload.Info) error
	// List returns information on the workload with the id on the unit.
	List(ids ...string) ([]workload.Result, error)
	// Settatus sets the status for the workload with the given id on the unit.
	SetStatus(id, status string) error
	// LookUp returns the payload ID for the given name/rawID pair.
	LookUp(name, rawID string) (string, error)
	// Untrack removes the information for the workload with the given id.
	Untrack(id string) error
}

// UnitFacade serves workload-specific API methods.
type UnitFacade struct {
	// State exposes the workload aspect of Juju's state.
	State UnitWorkloads
}

// NewUnitFacade builds a new facade for the given State.
func NewUnitFacade(st UnitWorkloads) *UnitFacade {
	return &UnitFacade{State: st}
}

// Track stores a workload to be tracked in state.
func (a UnitFacade) Track(args internal.TrackArgs) (internal.WorkloadResults, error) {
	logger.Debugf("tracking %d workloads from API", len(args.Workloads))

	var r internal.WorkloadResults
	for _, apiWorkload := range args.Workloads {
		info := internal.API2Workload(apiWorkload)
		logger.Debugf("tracking workload from API: %#v", info)

		id, err := a.track(info)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (a UnitFacade) track(info workload.Info) (string, error) {
	if err := a.State.Track(info); err != nil {
		return "", errors.Trace(err)
	}
	id, err := a.State.LookUp(info.Name, info.Details.ID)
	if err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

// List builds the list of workload being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// workloads for the unit are returned.
func (a UnitFacade) List(args params.Entities) (internal.WorkloadResults, error) {
	if len(args.Entities) == 0 {
		return a.listAll()
	}

	var ids []string
	for _, entity := range args.Entities {
		id, err := internal.API2ID(entity.Tag)
		if err != nil {
			return internal.WorkloadResults{}, errors.Trace(err)
		}
		ids = append(ids, id)
	}

	results, err := a.State.List(ids...)
	if err != nil {
		return internal.WorkloadResults{}, errors.Trace(err)
	}

	var r internal.WorkloadResults
	for _, result := range results {
		res := internal.Result2api(result)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (a UnitFacade) listAll() (internal.WorkloadResults, error) {
	var r internal.WorkloadResults

	results, err := a.State.List()
	if err != nil {
		return r, errors.Trace(err)
	}

	for _, result := range results {
		wl := *result.Workload
		id, err := a.State.LookUp(wl.Name, wl.Details.ID)
		if err != nil {
			logger.Errorf("failed to look up ID for %q: %v", wl.ID(), err)
			id = ""
		}
		apiwl := internal.Workload2api(wl)

		res := internal.NewWorkloadResult(id, nil)
		res.Workload = &apiwl
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// LookUp identifies the workload with the provided name and raw ID.
func (a UnitFacade) LookUp(args internal.LookUpArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		id, err := a.State.LookUp(arg.Name, arg.ID)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a workload.
func (a UnitFacade) SetStatus(args internal.SetStatusArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		id, err := internal.API2ID(arg.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = a.State.SetStatus(id, arg.Status)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified workload as no longer being tracked.
func (a UnitFacade) Untrack(args params.Entities) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, entity := range args.Entities {
		id, err := internal.API2ID(entity.Tag)
		if err != nil {
			return r, errors.Trace(err)
		}

		err = a.State.Untrack(id)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}
