// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"context"

	"github.com/juju/errors"

	api "github.com/juju/juju/api/client/payloads"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

// payloadBackend exposes the State functionality for payloads in a model.
type payloadBackend interface {
	// ListAll returns information on the payload with the id on the unit.
	ListAll() ([]payloads.FullPayloadInfo, error)
}

// API serves payload-specific API methods.
type API struct {
	// State exposes the payload aspect of Juju's state.
	backend payloadBackend
}

// NewAPI builds a new facade for the given backend.
func NewAPI(backend payloadBackend) *API {
	return &API{backend: backend}
}

// List builds the list of payloads being tracked for
// the given unit and IDs. If no IDs are provided then all tracked
// payloads for the unit are returned.
func (a API) List(ctx context.Context, args params.PayloadListArgs) (params.PayloadListResults, error) {
	var r params.PayloadListResults

	payloadInfo, err := a.backend.ListAll()
	if err != nil {
		return r, errors.Trace(err)
	}

	filters, err := payloads.BuildPredicatesFor(args.Patterns)
	if err != nil {
		return r, errors.Trace(err)
	}
	payloadInfo = payloads.Filter(payloadInfo, filters...)

	for _, payload := range payloadInfo {
		apiInfo := api.Payload2api(payload)
		r.Results = append(r.Results, apiInfo)
	}
	return r, nil
}
