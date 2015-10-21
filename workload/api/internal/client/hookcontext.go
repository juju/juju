// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"github.com/juju/names"

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

// TODO(ericsnow) Move these two to helpers.go.

// Track calls the Track API server method.
func (c HookContextClient) Track(workloads ...workload.Info) ([]workload.Result, error) {
	var args internal.TrackArgs
	for _, wl := range workloads {
		arg := internal.Workload2api(wl)
		args.Workloads = append(args.Workloads, arg)
	}

	var rs internal.WorkloadResults
	if err := c.FacadeCall("Track", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	results, err := internal.API2Results(rs, len(workloads))
	if err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
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

	var args internal.ListArgs
	for _, id := range ids {
		arg := names.NewPayloadTag(id)
		args.IDs = append(args.IDs, arg)
	}

	var rs internal.WorkloadResults
	if err := c.FacadeCall("List", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	size := len(fullIDs)
	if size == 0 {
		size = len(rs.Results)
	}
	results, err := internal.API2Results(rs, size)
	if err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
}

// LookUp calls the LookUp API server method.
func (c HookContextClient) LookUp(fullIDs ...string) ([]workload.Result, error) {
	// Unlike List(), LookUp doesn't fall back to looking up all IDs.
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

	var rs internal.WorkloadResults
	if err := c.FacadeCall("LookUp", &args, &rs); err != nil {
		return nil, err
	}

	results, err := internal.API2Results(rs, len(fullIDs))
	if err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
}

// SetStatus calls the SetStatus API server method.
func (c HookContextClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var args internal.SetStatusArgs
	for _, id := range ids {
		arg := internal.SetStatusArg{
			ID:     names.NewPayloadTag(id),
			Status: status,
		}
		args.Args = append(args.Args, arg)
	}

	var rs internal.WorkloadResults
	if err := c.FacadeCall("SetStatus", &args, &rs); err != nil {
		return nil, err
	}

	results, err := internal.API2Results(rs, len(fullIDs))
	if err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
}

// Untrack calls the Untrack API server method.
func (c HookContextClient) Untrack(fullIDs ...string) ([]workload.Result, error) {
	logger.Tracef("Calling untrack API: %q", fullIDs)

	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var args internal.UntrackArgs
	for _, id := range ids {
		arg := names.NewPayloadTag(id)
		args.IDs = append(args.IDs, arg)
	}

	var rs internal.WorkloadResults
	if err := c.FacadeCall("Untrack", &args, &rs); err != nil {
		return nil, err
	}

	results, err := internal.API2Results(rs, len(fullIDs))
	if err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
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
