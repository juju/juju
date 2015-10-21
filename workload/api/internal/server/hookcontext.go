// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver/common import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
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
		var res internal.WorkloadResult
		err := a.State.Track(info)
		id := ""
		if err == nil {
			id, err = a.State.LookUp(info.Name, info.Details.ID)
		}
		res.ID = names.NewPayloadTag(id)
		if err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}

		r.Results = append(r.Results, res)
	}
	return r, nil
}

func (a HookContextAPI) listAll() (internal.WorkloadResults, error) {
	var r internal.WorkloadResults

	results, err := a.State.List()
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}

	for _, result := range results {
		wl := *result.Workload
		id, err := a.State.LookUp(wl.Name, wl.Details.ID)
		if err != nil {
			logger.Errorf("failed to look up ID for %q: %v", wl.ID(), err)
			id = ""
		}

		apiwl := internal.Workload2api(wl)
		res := internal.WorkloadResult{
			ID:       names.NewPayloadTag(id),
			Workload: &apiwl,
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
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
		var r internal.WorkloadResults
		r.Error = common.ServerError(err)
		return r, nil
	}

	r := internal.Results2api(results)
	return r, nil
}

// LookUp identifies the workload with the provided name and raw ID.
func (a HookContextAPI) LookUp(args internal.LookUpArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		var res internal.WorkloadResult

		id, err := a.State.LookUp(arg.Name, arg.ID)
		if err != nil {
			if errors.IsNotFound(err) {
				res.NotFound = true
			}
			res.Error = common.ServerError(err)
			r.Error = common.ServerError(api.BulkFailure)
		} else {
			res.ID = names.NewPayloadTag(id)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a workload.
func (a HookContextAPI) SetStatus(args internal.SetStatusArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, arg := range args.Args {
		res := internal.WorkloadResult{
			ID: arg.ID,
		}
		id := arg.ID.Id()

		if err := a.State.SetStatus(id, arg.Status); err != nil {
			if errors.IsNotFound(err) {
				res.NotFound = true
			}
			res.Error = common.ServerError(err)
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified workload as no longer being tracked.
func (a HookContextAPI) Untrack(args internal.UntrackArgs) (internal.WorkloadResults, error) {
	var r internal.WorkloadResults
	for _, id := range args.IDs {
		res := internal.WorkloadResult{
			ID: id,
		}

		if err := a.State.Untrack(id.Id()); err != nil {
			if errors.IsNotFound(err) {
				res.NotFound = true
			}
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}
