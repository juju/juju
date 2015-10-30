// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Eliminate the apiserver/common import if possible.
// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
)

// NewPayloadResult builds a new WorkloadResult from the provided tag
// and error. NotFound is also set based on the error.
func NewPayloadResult(id string, err error) PayloadResult {
	result := workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: errors.IsNotFound(err),
		Error:    err,
	}
	return Result2api(result)
}

// API2Result converts the API result to a workload.Result.
func API2Result(r PayloadResult) (workload.Result, error) {
	result := workload.Result{
		NotFound: r.NotFound,
	}

	id, err := API2ID(r.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.ID = id

	if r.Payload != nil {
		pl, err := api.API2Payload(*r.Payload)
		if err != nil {
			return result, errors.Trace(err)
		}
		info := pl.AsWorkload()
		result.Workload = &info
	}

	if r.Error != nil {
		result.Error, _ = common.RestoreError(r.Error)
	}

	return result, nil
}

// Result2api converts the workload.Result into a WorkloadResult.
func Result2api(result workload.Result) PayloadResult {
	res := PayloadResult{
		NotFound: result.NotFound,
	}

	if result.ID != "" {
		res.Tag = names.NewPayloadTag(result.ID).String()
	}

	if result.Workload != nil {
		fullPayload := workload.FullPayloadInfo{Payload: result.Workload.AsPayload()}
		pl := api.Payload2api(fullPayload)
		res.Payload = &pl
	}

	if result.Error != nil {
		res.Error = common.ServerError(result.Error)
	}

	return res
}

// API2ID converts the given tag string into a payload ID.
func API2ID(tagStr string) (string, error) {
	if tagStr == "" {
		return tagStr, nil
	}
	tag, err := names.ParsePayloadTag(tagStr)
	if err != nil {
		return "", errors.Trace(err)
	}
	return tag.Id(), nil
}

// Infos2TrackArgs converts the provided workload info into arguments
// for the Track API endpoint.
func Infos2TrackArgs(workloads []workload.Info) TrackArgs {
	var args TrackArgs
	for _, wl := range workloads {
		fullPayload := workload.FullPayloadInfo{Payload: wl.AsPayload()}
		arg := api.Payload2api(fullPayload)
		args.Payloads = append(args.Payloads, arg)
	}
	return args
}

// IDs2ListArgs converts the provided workload IDs into arguments
// for the List API endpoint.
func IDs2ListArgs(ids []string) params.Entities {
	return ids2args(ids)
}

// FullIDs2LookUpArgs converts the provided workload "full" IDs into arguments
// for the LookUp API endpoint.
func FullIDs2LookUpArgs(fullIDs []string) LookUpArgs {
	var args LookUpArgs
	for _, fullID := range fullIDs {
		name, rawID := workload.ParseID(fullID)
		args.Args = append(args.Args, LookUpArg{
			Name: name,
			ID:   rawID,
		})
	}
	return args
}

// IDs2SetStatusArgs converts the provided workload IDs into arguments
// for the SetStatus API endpoint.
func IDs2SetStatusArgs(ids []string, status string) SetStatusArgs {
	var args SetStatusArgs
	for _, id := range ids {
		arg := SetStatusArg{
			Status: status,
		}
		arg.Tag = names.NewPayloadTag(id).String()
		args.Args = append(args.Args, arg)
	}
	return args
}

// IDs2UntrackArgs converts the provided workload IDs into arguments
// for the Untrack API endpoint.
func IDs2UntrackArgs(ids []string) params.Entities {
	return ids2args(ids)
}

func ids2args(ids []string) params.Entities {
	var args params.Entities
	for _, id := range ids {
		tag := names.NewPayloadTag(id).String()
		args.Entities = append(args.Entities, params.Entity{
			Tag: tag,
		})
	}
	return args
}
