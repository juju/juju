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
	List(ids ...string) ([]workload.Info, error)
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

	r := internal.WorkloadResults{}
	for _, apiWorkload := range args.Workloads {
		info := internal.API2Workload(apiWorkload)
		logger.Debugf("tracking workload from API: %#v", info)
		res := internal.WorkloadResult{
			ID: internal.FullID2api(info.ID()),
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

	var ids, stateIDs []string
	for _, id := range args.IDs {
		fullID := internal.API2FullID(id)
		ids = append(ids, fullID)

		name, rawID := workload.ParseID(fullID)
		stateID, err := a.State.LookUp(name, rawID)
		if err != nil {
			logger.Errorf("could not look up payload ID for %q", fullID)
			continue
		}
		stateIDs = append(stateIDs, stateID)
	}

	workloads, err := a.State.List(stateIDs...)
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
			ID: internal.FullID2api(id),
		}

		found := false
		for _, wl := range workloads {
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
		res := internal.WorkloadResult{
			ID: arg.ID,
		}
		fullID := internal.API2FullID(arg.ID)

		name, rawID := workload.ParseID(fullID)
		stateID, err := a.State.LookUp(name, rawID)
		if err == nil {
			err = a.State.SetStatus(stateID, arg.Status)
		}
		if err != nil {
			res.Error = common.ServerError(err)
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// LookUp identifies the workload with the provided name and raw ID.
func (a HookContextAPI) LookUp(args internal.LookUpArgs) (internal.LookUpResults, error) {
	var r internal.LookUpResults
	for _, arg := range args.Args {
		var res internal.LookUpResult

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

// Untrack marks the identified workload as no longer being tracked.
func (a HookContextAPI) Untrack(args internal.UntrackArgs) (internal.WorkloadResults, error) {
	r := internal.WorkloadResults{}
	for _, id := range args.IDs {
		res := internal.WorkloadResult{
			ID: id,
		}
		fullID := internal.API2FullID(id)
		name, rawID := workload.ParseID(fullID)

		stateID, err := a.State.LookUp(name, rawID)
		if err == nil {
			err = a.State.Untrack(stateID)
		}
		if err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}
