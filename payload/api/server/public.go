// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
)

// EnvPayloads exposes the State functionality for payloads in an env.
type EnvPayloads interface {
	// ListAll returns information on the payload with the id on the unit.
	ListAll() ([]payload.FullPayloadInfo, error)
}

// PublicAPI serves payload-specific API methods.
type PublicAPI struct {
	// State exposes the payload aspect of Juju's state.
	State EnvPayloads
}

// NewHookContextAPI builds a new facade for the given State.
func NewPublicAPI(st EnvPayloads) *PublicAPI {
	return &PublicAPI{State: st}
}

// List builds the list of payloads being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (a PublicAPI) List(args api.EnvListArgs) (api.EnvListResults, error) {
	var r api.EnvListResults

	payloads, err := a.State.ListAll()
	if err != nil {
		return r, errors.Trace(err)
	}

	filters, err := payload.BuildPredicatesFor(args.Patterns)
	if err != nil {
		return r, errors.Trace(err)
	}
	payloads = payload.Filter(payloads, filters...)

	for _, payload := range payloads {
		apiInfo := api.Payload2api(payload)
		r.Results = append(r.Results, apiInfo)
	}
	return r, nil
}
