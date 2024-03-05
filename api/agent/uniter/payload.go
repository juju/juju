// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/payloads"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

// PayloadFacadeClient provides methods for interacting with Juju's internal
// RPC API, relative to payloads.
type PayloadFacadeClient struct {
	FacadeCaller
}

// NewPayloadFacadeClient builds a new payload API client.
func NewPayloadFacadeClient(caller base.APICaller) *PayloadFacadeClient {
	facadeCaller := base.NewFacadeCaller(caller, "PayloadsHookContext")
	return &PayloadFacadeClient{FacadeCaller: facadeCaller}
}

// Track calls the Track API server method.
func (c PayloadFacadeClient) Track(payloads ...payloads.Payload) ([]payloads.Result, error) {
	args := payloads2TrackArgs(payloads)

	var rs params.PayloadResults
	if err := c.FacadeCall(context.TODO(), "Track", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	return api2results(rs)
}

// List calls the List API server method.
func (c PayloadFacadeClient) List(fullIDs ...string) ([]payloads.Result, error) {
	var ids []string
	if len(fullIDs) > 0 {
		actual, err := c.lookUp(fullIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ids = actual
	}
	args := ids2Args(ids)

	var rs params.PayloadResults
	if err := c.FacadeCall(context.TODO(), "List", &args, &rs); err != nil {
		return nil, errors.Trace(err)
	}

	return api2results(rs)
}

// LookUp calls the LookUp API server method.
func (c PayloadFacadeClient) LookUp(fullIDs ...string) ([]payloads.Result, error) {
	if len(fullIDs) == 0 {
		// Unlike List(), LookUp doesn't fall back to looking up all IDs.
		return nil, nil
	}
	args := fullIDs2LookUpArgs(fullIDs)

	var rs params.PayloadResults
	if err := c.FacadeCall(context.TODO(), "LookUp", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs)
}

// SetStatus calls the SetStatus API server method.
func (c PayloadFacadeClient) SetStatus(status string, fullIDs ...string) ([]payloads.Result, error) {
	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := ids2SetStatusArgs(ids, status)

	var rs params.PayloadResults
	if err := c.FacadeCall(context.TODO(), "SetStatus", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs)
}

// Untrack calls the Untrack API server method.
func (c PayloadFacadeClient) Untrack(fullIDs ...string) ([]payloads.Result, error) {
	ids, err := c.lookUp(fullIDs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := ids2Args(ids)

	var rs params.PayloadResults
	if err := c.FacadeCall(context.TODO(), "Untrack", &args, &rs); err != nil {
		return nil, err
	}

	return api2results(rs)
}

func (c PayloadFacadeClient) lookUp(fullIDs []string) ([]string, error) {
	results, err := c.LookUp(fullIDs...)
	if err != nil {
		return nil, errors.Annotate(err, "while looking up IDs")
	}

	var ids []string
	for _, result := range results {
		if result.Error != nil {
			// TODO(ericsnow) Do not short-circuit?
			return nil, errors.Annotate(result.Error, "while looking up IDs")
		}
		ids = append(ids, result.ID)
	}
	return ids, nil
}

func api2results(rs params.PayloadResults) ([]payloads.Result, error) {
	var results []payloads.Result
	for _, r := range rs.Results {
		result, err := api2Result(r)
		if err != nil {
			// This should not happen; we safely control the result.
			return nil, errors.Trace(err)
		}
		results = append(results, result)
	}
	return results, nil
}

// api2Result converts the API result to a payloads.Result.
func api2Result(r params.PayloadResult) (payloads.Result, error) {
	result := payloads.Result{
		NotFound: r.NotFound,
	}

	id, err := api.API2ID(r.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.ID = id

	if r.Payload != nil {
		pl, err := api.API2Payload(*r.Payload)
		if err != nil {
			return result, errors.Trace(err)
		}
		result.Payload = &pl
	}

	if r.Error != nil {
		result.Error = apiservererrors.RestoreError(r.Error)
	}

	return result, nil
}

// payloads2TrackArgs converts the provided payload info into arguments
// for the Track API endpoint.
func payloads2TrackArgs(payloadInfo []payloads.Payload) params.TrackPayloadArgs {
	var args params.TrackPayloadArgs
	for _, pl := range payloadInfo {
		fullPayload := payloads.FullPayloadInfo{Payload: pl}
		arg := api.Payload2api(fullPayload)
		args.Payloads = append(args.Payloads, arg)
	}
	return args
}

// fullIDs2LookUpArgs converts the provided payload "full" IDs into arguments
// for the LookUp API endpoint.
func fullIDs2LookUpArgs(fullIDs []string) params.LookUpPayloadArgs {
	var args params.LookUpPayloadArgs
	for _, fullID := range fullIDs {
		name, rawID := payloads.ParseID(fullID)
		args.Args = append(args.Args, params.LookUpPayloadArg{
			Name: name,
			ID:   rawID,
		})
	}
	return args
}

// ids2SetStatusArgs converts the provided payload IDs into arguments
// for the SetStatus API endpoint.
func ids2SetStatusArgs(ids []string, status string) params.SetPayloadStatusArgs {
	var args params.SetPayloadStatusArgs
	for _, id := range ids {
		arg := params.SetPayloadStatusArg{
			Status: status,
		}
		arg.Tag = names.NewPayloadTag(id).String()
		args.Args = append(args.Args, arg)
	}
	return args
}

func ids2Args(ids []string) params.Entities {
	var args params.Entities
	for _, id := range ids {
		tag := names.NewPayloadTag(id).String()
		args.Entities = append(args.Entities, params.Entity{
			Tag: tag,
		})
	}
	return args
}
