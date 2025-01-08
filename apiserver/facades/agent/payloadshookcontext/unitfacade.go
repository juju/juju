// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadshookcontext

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
)

// NewHookContextFacadeV1 returns a new payloads hook context facade for
// the State and Unit given. It is used for facade registration.
func NewHookContextFacadeV1() (*UnitFacadeV1, error) {
	return NewUnitFacadeV1(), nil
}

// UnitFacadeV1 serves payload-specific API methods.
type UnitFacadeV1 struct{}

// NewUnitFacadeV1 builds a new facade for the given backend.
func NewUnitFacadeV1() *UnitFacadeV1 {
	return &UnitFacadeV1{}
}

// Track stores a payload to be tracked in state.
func (uf UnitFacadeV1) Track(ctx context.Context, args params.TrackPayloadArgs) (params.PayloadResults, error) {
	var r params.PayloadResults
	for range args.Payloads {
		r.Results = append(r.Results, params.PayloadResult{
			NotFound: true,
		})
	}
	return r, nil
}

// List builds the list of payload being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (uf UnitFacadeV1) List(ctx context.Context, args params.Entities) (params.PayloadResults, error) {
	if len(args.Entities) == 0 {
		return params.PayloadResults{}, nil
	}

	var r params.PayloadResults
	for _, arg := range args.Entities {
		r.Results = append(r.Results, params.PayloadResult{
			Entity: params.Entity{
				Tag: arg.Tag,
			},
			NotFound: true,
		})
	}
	return r, nil
}

// LookUp identifies the payload with the provided name and raw ID.
func (uf UnitFacadeV1) LookUp(ctx context.Context, args params.LookUpPayloadArgs) (params.PayloadResults, error) {
	var r params.PayloadResults
	for range args.Args {
		r.Results = append(r.Results, params.PayloadResult{
			NotFound: true,
		})
	}
	return r, nil
}

// SetStatus sets the raw status of a payload.
func (uf UnitFacadeV1) SetStatus(ctx context.Context, args params.SetPayloadStatusArgs) (params.PayloadResults, error) {
	var r params.PayloadResults
	for _, arg := range args.Args {
		r.Results = append(r.Results, params.PayloadResult{
			Entity: params.Entity{
				Tag: arg.Tag,
			},
			NotFound: true,
		})
	}
	return r, nil
}

// Untrack marks the identified payload as no longer being tracked.
func (uf UnitFacadeV1) Untrack(ctx context.Context, args params.Entities) (params.PayloadResults, error) {
	var r params.PayloadResults
	for _, arg := range args.Entities {
		r.Results = append(r.Results, params.PayloadResult{
			Entity: params.Entity{
				Tag: arg.Tag,
			},
			NotFound: arg.Tag != "",
		})
	}
	return r, nil
}

// parsePayloadTag converts the given payload tag string into a payload ID.
// Example: "payload-foobar" -> "foobar"
func parsePayloadTag(tagStr string) (string, error) {
	if tagStr == "" {
		return tagStr, nil
	}
	tag, err := names.ParsePayloadTag(tagStr)
	if err != nil {
		return "", errors.Trace(err)
	}
	return tag.Id(), nil
}

// NewHookContextFacadeV2 returns a new payloads hook context facade for
// the State and Unit given. It is used for facade registration.
func NewHookContextFacadeV2() (*UnitFacadeV2, error) {
	return NewUnitFacadeV2(), nil
}

// UnitFacadeV2 serves payload-specific API methods.
type UnitFacadeV2 struct{}

// NewUnitFacadeV2 builds a new facade for the given backend.
func NewUnitFacadeV2() *UnitFacadeV2 {
	return &UnitFacadeV2{}
}
