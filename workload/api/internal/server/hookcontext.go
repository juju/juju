// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"

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

// HookContextAPI serves workload-specific API methods.
type HookContextAPI struct {
	// State exposes the workload aspect of Juju's state.
	State UnitWorkloads
}

// NewHookContextAPI builds a new facade for the given State.
func NewHookContextAPI(st UnitWorkloads) *HookContextAPI {
	return &HookContextAPI{State: st}
}

// Track stores a workload to be tracked in state.
func (a HookContextAPI) Track(args internal.TrackArgs) (internal.WorkloadResults, error) {
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

func (a HookContextAPI) track(info workload.Info) (string, error) {
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
func (a HookContextAPI) List(args internal.ListArgs) (internal.WorkloadResults, error) {
	if len(args.IDs) == 0 {
		return a.listAll()
	}

	var ids []string
	for _, id := range args.IDs {
		ids = append(ids, id.Id())
	}

	results, err := a.State.List(ids...)
	if err != nil {
		return internal.WorkloadResults{}, errors.Cause(err)
	}

	var r internal.WorkloadResults
	for _, result := range results {
		r.Results = append(r.Results, internal.Result2api(result))
	}
	return r, nil
}

func (a HookContextAPI) listAll() (internal.WorkloadResults, error) {
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
func (a HookContextAPI) LookUp(args internal.LookUpArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		id, err := a.State.LookUp(arg.Name, arg.ID)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a workload.
func (a HookContextAPI) SetStatus(args internal.SetStatusArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		id := arg.ID.Id()
		err := a.State.SetStatus(id, arg.Status)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified workload as no longer being tracked.
func (a HookContextAPI) Untrack(args internal.UntrackArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, tag := range args.IDs {
		id := tag.Id()
		err := a.State.Untrack(id)
		res := internal.NewWorkloadResult(id, err)
		r.Results = append(r.Results, res)
	}
	return r, nil
}
