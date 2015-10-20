// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/internal"
)

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// HookContextClient provides methods for interacting with Juju's internal
// RPC API, relative to workloads.
type HookContextClient struct {
	facadeCaller
}

// NewHookContextClient builds a new workload API client.
func NewHookContextClient(caller facadeCaller) HookContextClient {
	return HookContextClient{caller}
}

// Track calls the Track API server method.
func (c HookContextClient) Track(workloads ...workload.Info) ([]string, error) {
	workloadArgs := make([]internal.Workload, len(workloads))
	for i, wl := range workloads {
		workloadArgs[i] = internal.Workload2api(wl)
	}

	var result internal.WorkloadResults

	args := internal.TrackArgs{Workloads: workloadArgs}
	if err := c.FacadeCall("Track", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Errorf(result.Error.GoString())
	}

	ids := make([]string, len(result.Results))
	for i, r := range result.Results {
		if r.Error != nil {
			return nil, errors.Errorf(r.Error.GoString())
		}
		ids[i] = internal.API2FullID(r.ID)
	}
	return ids, nil
}

// List calls the List API server method.
func (c HookContextClient) List(fullIDs ...string) ([]workload.Info, error) {
	var result internal.ListResults

	var args internal.ListArgs
	for _, fullID := range fullIDs {
		arg := internal.FullID2api(fullID)
		args.IDs = append(args.IDs, arg)
	}

	if err := c.FacadeCall("List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	var notFound []string
	workloads := make([]workload.Info, len(result.Results))
	for i, presult := range result.Results {
		if presult.NotFound {
			id := internal.API2FullID(presult.ID)
			notFound = append(notFound, id)
			continue
		}
		if presult.Error != nil {
			return workloads, errors.Errorf(presult.Error.GoString())
		}
		pp := internal.API2Workload(presult.Info)
		workloads[i] = pp
	}
	if len(notFound) > 0 {
		return workloads, errors.NotFoundf("%v", notFound)
	}
	return workloads, nil
}

// TODO(ericsnow) Return []workload.Result.

// LookUp calls the LookUp API server method.
func (c HookContextClient) LookUp(fullIDs ...string) ([]string, error) {
	if len(fullIDs) == 0 {
		return nil, nil
	}

	var args internal.LookUpArgs
	for _, fullID := range fullIDs {
		name, rawID := workload.ParseID(fullID)
		args.Args = append(args.Args, internal.LookUpArg{
			Name: name,
			ID:   rawID,
		})
	}

	var res internal.LookUpResults
	if err := c.FacadeCall("LookUp", &args, &res); err != nil {
		return nil, err
	}
	if res.Error != nil && len(res.Results) != len(fullIDs) {
		return nil, errors.Errorf(res.Error.GoString())
	}

	var ids []string
	for _, r := range res.Results {
		if r.Error != nil {
			// TODO(ericsnow) preserve the error
			ids = append(ids, "")
			continue
		}

		id := r.ID.Id()
		ids = append(ids, id)
	}

	if res.Error != nil {
		return ids, errors.Errorf(res.Error.GoString())
	}
	return ids, nil
}

// SetStatus calls the SetStatus API server method.
func (c HookContextClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	statusArgs := make([]internal.SetStatusArg, len(fullIDs))
	for i, fullID := range fullIDs {
		statusArgs[i] = internal.SetStatusArg{
			ID:     internal.FullID2api(fullID),
			Status: status,
		}
	}
	args := internal.SetStatusArgs{Args: statusArgs}

	res := internal.WorkloadResults{}
	if err := c.FacadeCall("SetStatus", &args, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, errors.Errorf(res.Error.GoString())
	}

	var errs []workload.Result
	if len(res.Results) > 0 {
		errs = make([]workload.Result, len(res.Results))
		for i, r := range res.Results {
			fullID := internal.API2FullID(r.ID)
			p := workload.Result{FullID: fullID}
			if r.Error != nil {
				p.Err = r.Error
			}
			errs[i] = p
		}
	}

	return errs, nil
}

// Untrack calls the Untrack API server method.
func (c HookContextClient) Untrack(fullIDs ...string) ([]workload.Result, error) {
	logger.Tracef("Calling untrack API: %q", fullIDs)

	var args internal.UntrackArgs
	for _, fullID := range fullIDs {
		arg := internal.FullID2api(fullID)
		args.IDs = append(args.IDs, arg)
	}

	var res internal.WorkloadResults
	if err := c.FacadeCall("Untrack", &args, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, errors.Errorf(res.Error.GoString())
	}
	var errs []workload.Result
	if len(res.Results) > 0 {
		errs = make([]workload.Result, len(res.Results))
		for i, r := range res.Results {
			fullID := internal.API2FullID(r.ID)
			p := workload.Result{FullID: fullID}
			if r.Error != nil {
				p.Err = r.Error
			}
			errs[i] = p
		}
	}
	return errs, nil
}
