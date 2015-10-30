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

// UnitFacadeClient provides methods for interacting with Juju's internal
// RPC API, relative to workloads.
type UnitFacadeClient struct {
	facadeCaller
}

// NewUnitFacadeClient builds a new workload API client.
func NewUnitFacadeClient(caller facadeCaller) UnitFacadeClient {
	return UnitFacadeClient{caller}
}

// Track calls the Track API server method.
func (c UnitFacadeClient) Track(workloads ...workload.Info) ([]workload.Result, error) {
	args := internal.Infos2TrackArgs(workloads)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("Track", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	return api2results(rs)
}

// List calls the List API server method.
func (c UnitFacadeClient) List(fullIDs ...string) ([]workload.Result, error) {
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

	return api2results(rs)
}

// LookUp calls the LookUp API server method.
func (c UnitFacadeClient) LookUp(fullIDs ...string) ([]workload.Result, error) {
	if len(fullIDs) == 0 {
		// Unlike List(), LookUp doesn't fall back to looking up all IDs.
		return nil, nil
	}
	args := internal.FullIDs2LookUpArgs(fullIDs)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("LookUp", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs)
}

// SetStatus calls the SetStatus API server method.
func (c UnitFacadeClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := internal.IDs2SetStatusArgs(ids, status)

	var rs internal.WorkloadResults
	if err := c.FacadeCall("SetStatus", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs)
}

// Untrack calls the Untrack API server method.
func (c UnitFacadeClient) Untrack(fullIDs ...string) ([]workload.Result, error) {
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

	return api2results(rs)
}

func (c UnitFacadeClient) lookUp(fullIDs []string) ([]string, error) {
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

func api2results(rs internal.WorkloadResults) ([]workload.Result, error) {
	var results []workload.Result
	for _, r := range rs.Results {
		result, err := internal.API2Result(r)
		if err != nil {
			// This should not happen; we safely control the result.
			return nil, errors.Trace(err)
		}
		results = append(results, result)
	}
	return results, nil
}
