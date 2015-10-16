// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver/common import if possible.

import (
	"github.com/juju/errors"

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
	List(ids ...string) ([]workload.Info, error)
	// Settatus sets the status for the workload with the given id on the unit.
	SetStatus(docID, status string) error
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

	r := internal.WorkloadResults{}
	for _, apiWorkload := range args.Workloads {
		info := internal.API2Workload(apiWorkload)
		logger.Debugf("tracking workload from API: %#v", info)
		res := internal.WorkloadResult{
			ID: info.ID(),
		}
		if err := a.State.Track(info); err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}

		r.Results = append(r.Results, res)
	}
	return r, nil
}

// List builds the list of workload being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// workloads for the unit are returned.
func (a HookContextAPI) List(args internal.ListArgs) (internal.ListResults, error) {
	var r internal.ListResults

	ids := args.IDs
	workloads, err := a.State.List(ids...)
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}

	if len(ids) == 0 {
		for _, wl := range workloads {
			ids = append(ids, wl.ID())
		}
	}

	for _, id := range ids {
		res := internal.ListResult{
			ID: id,
		}

		found := false
		for _, wl := range workloads {
			workloadID := wl.Name
			if wl.Details.ID != "" {
				workloadID += "/" + wl.Details.ID
			}
			if id == wl.ID() {
				res.Info = internal.Workload2api(wl)
				found = true
				break
			}
		}
		if !found {
			res.NotFound = true
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetStatus sets the raw status of a workload.
func (a HookContextAPI) SetStatus(args internal.SetStatusArgs) (internal.WorkloadResults, error) {
	r := internal.WorkloadResults{}
	for _, arg := range args.Args {
		ID := workload.BuildID(arg.Class, arg.ID)
		res := internal.WorkloadResult{
			ID: ID,
		}
		err := a.State.SetStatus(ID, arg.Status)
		if err != nil {
			res.Error = common.ServerError(err)
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// Untrack marks the identified workload as no longer being tracked.
func (a HookContextAPI) Untrack(args internal.UntrackArgs) (internal.WorkloadResults, error) {
	r := internal.WorkloadResults{}
	for _, id := range args.IDs {
		res := internal.WorkloadResult{
			ID: id,
		}
		if err := a.State.Untrack(id); err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}
