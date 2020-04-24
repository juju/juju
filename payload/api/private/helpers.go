// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package private

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
)

// API2Result converts the API result to a payload.Result.
func API2Result(r params.PayloadResult) (payload.Result, error) {
	result := payload.Result{
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
		result.Error = common.RestoreError(r.Error)
	}

	return result, nil
}

// Payloads2TrackArgs converts the provided payload info into arguments
// for the Track API endpoint.
func Payloads2TrackArgs(payloads []payload.Payload) params.TrackPayloadArgs {
	var args params.TrackPayloadArgs
	for _, pl := range payloads {
		fullPayload := payload.FullPayloadInfo{Payload: pl}
		arg := api.Payload2api(fullPayload)
		args.Payloads = append(args.Payloads, arg)
	}
	return args
}

// IDs2ListArgs converts the provided payload IDs into arguments
// for the List API endpoint.
func IDs2ListArgs(ids []string) params.Entities {
	return ids2args(ids)
}

// FullIDs2LookUpArgs converts the provided payload "full" IDs into arguments
// for the LookUp API endpoint.
func FullIDs2LookUpArgs(fullIDs []string) params.LookUpPayloadArgs {
	var args params.LookUpPayloadArgs
	for _, fullID := range fullIDs {
		name, rawID := payload.ParseID(fullID)
		args.Args = append(args.Args, params.LookUpPayloadArg{
			Name: name,
			ID:   rawID,
		})
	}
	return args
}

// IDs2SetStatusArgs converts the provided payload IDs into arguments
// for the SetStatus API endpoint.
func IDs2SetStatusArgs(ids []string, status string) params.SetPayloadStatusArgs {
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

// IDs2UntrackArgs converts the provided payload IDs into arguments
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
