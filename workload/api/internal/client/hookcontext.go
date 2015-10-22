// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

// TODO(ericsnow) Eliminate the params import if possible.

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
func (c HookContextClient) Track(workloads ...workload.Info) ([]workload.Result, error) {
	args := internal.Infos2TrackArgs(workloads)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("Track", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	return api2results(rs), nil
}

// List calls the List API server method.
func (c HookContextClient) List(fullIDs ...string) ([]workload.Result, error) {
	var ids []string
	if len(fullIDs) > 0 {
		actual, err := c.lookUp(fullIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ids = actual
	}
	args := internal.IDs2ListArgs(ids)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("List", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	return api2results(rs), nil
}

// LookUp calls the LookUp API server method.
func (c HookContextClient) LookUp(fullIDs ...string) ([]workload.Result, error) {
	if len(fullIDs) == 0 {
		// Unlike List(), LookUp doesn't fall back to looking up all IDs.
		return nil, nil
	}
	args := internal.FullIDs2LookUpArgs(fullIDs)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("LookUp", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs), nil
}

// SetStatus calls the SetStatus API server method.
func (c HookContextClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := internal.IDs2SetStatusArgs(ids, status)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("SetStatus", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs), nil
}

// Untrack calls the Untrack API server method.
func (c HookContextClient) Untrack(fullIDs ...string) ([]workload.Result, error) {
	logger.Tracef("Calling untrack API: %q", fullIDs)

	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := internal.IDs2UntrackArgs(ids)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("Untrack", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs), nil
}

func (c HookContextClient) lookUp(fullIDs []string) ([]string, error) {
	results, err := c.LookUp(fullIDs...)
	if err != nil {
		return nil, errors.Annotate(err, "while looking up IDs")
	}

	var ids []string
	for _, result := range results {
		if result.Error != nil && !result.NotFound {
			// TODO(ericsnow) Do not short-circuit?
			return nil, errors.Annotate(result.Error, "while looking up IDs")
		}
		ids = append(ids, result.ID)
	}
	return ids, nil
}

func api2results(rs internal.WorkloadResults) []workload.Result {
	var results []workload.Result
	for _, r := range rs.Results {
		results = append(results, api2result(r))
	}
	return results
}

func api2result(r internal.WorkloadResult) workload.Result {
	// We control the result safely so we can ignore the error.
	result, _ := internal.API2Result(r)
	return result
}
