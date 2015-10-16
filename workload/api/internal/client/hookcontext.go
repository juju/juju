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
		ids[i] = r.ID
	}
	return ids, nil
}

// List calls the List API server method.
func (c HookContextClient) List(fullIDs ...string) ([]workload.Info, error) {
	var result internal.ListResults

	args := internal.ListArgs{IDs: fullIDs}
	if err := c.FacadeCall("List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	var notFound []string
	workloads := make([]workload.Info, len(result.Results))
	for i, presult := range result.Results {
		if presult.NotFound {
			notFound = append(notFound, presult.ID)
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

// SetStatus calls the SetStatus API server method.
func (c HookContextClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	statusArgs := make([]internal.SetStatusArg, len(fullIDs))
	for i, fullID := range fullIDs {
		class, id := workload.ParseID(fullID)
		statusArgs[i] = internal.SetStatusArg{
			Class:  class,
			ID:     id,
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
			p := workload.Result{ID: r.ID}
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
	args := internal.UntrackArgs{IDs: fullIDs}
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
			p := workload.Result{ID: r.ID}
			if r.Error != nil {
				p.Err = r.Error
			}
			errs[i] = p
		}
	}
	return errs, nil
}
