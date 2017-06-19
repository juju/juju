// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/state"
)

// PayloadsAPI serves payload-specific API methods.
type PayloadsAPI struct {
	backend payloadsStateInterface
}

// newPayloadsAPI builds a new facade for the given backend.
func newPayloadsAPI(st *state.State, unit *state.Unit) (*PayloadsAPI, error) {
	unitPayloadsSt, err := st.UnitPayloads(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &PayloadsAPI{getPayloadsState(unitPayloadsSt)}, nil
}

func (uf PayloadsAPI) TrackPayloads(args params.TrackPayloadsParams) (params.ErrorResults, error) {
	// do i care about permissions to access unit?
	if len(args.Payloads) == 0 {
		return params.ErrorResults{}, nil
	}
	logger.Debugf("tracking %d payloads from API", len(args.Payloads))
	r := make([]params.ErrorResult, len(args.Payloads))
	for i, p := range args.Payloads {
		pl := payloadFromParams(p)
		logger.Debugf("tracking payload from API: %#v", pl)
		if err := uf.backend.Track(pl); err != nil {
			r[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: r}, nil
}

// UntrackPayloads stops tracking specified payloads.
func (uf PayloadsAPI) UntrackPayloads(args params.UntrackPayloadsParams) (params.ErrorResults, error) {
	// do i care about permissions to access unit?
	logger.Debugf("untracking %d payloads from API", len(args.Payloads))
	if len(args.Payloads) == 0 {
		return params.ErrorResults{}, nil
	}
	r := make([]params.ErrorResult, len(args.Payloads))
	for i, p := range args.Payloads {
		logger.Debugf("untracking payload from API: %#v", p.Class)
		if err := uf.backend.Untrack(p.Class); err != nil {
			r[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: r}, nil
}

// SetPayloadsStatus sets the status of payloads.
func (uf PayloadsAPI) SetPayloadsStatus(args params.PayloadsStatusParams) (params.ErrorResults, error) {
	if len(args.Payloads) == 0 {
		return params.ErrorResults{}, nil
	}

	r := make([]params.ErrorResult, len(args.Payloads))
	for i, p := range args.Payloads {
		if err := uf.backend.SetStatus(p.Class, p.Status); err != nil {
			r[i].Error = common.ServerError(err)
		}
	}
	return params.ErrorResults{Results: r}, nil
}

func payloadFromParams(p params.TrackPayloadParams) payload.Payload {
	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)

	return payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: p.Class,
			Type: p.Type,
		},
		ID:     p.ID,
		Status: p.Status,
		Labels: labels,
	}
}
