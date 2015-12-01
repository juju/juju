// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
)

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

type rawAPI interface {
	facadeCaller
	io.Closer
}

// PublicClient provides methods for interacting with Juju's public
// RPC API, relative to payloads.
type PublicClient struct {
	rawAPI
}

// NewPublicClient builds a new payload API client.
func NewPublicClient(raw rawAPI) PublicClient {
	return PublicClient{
		rawAPI: raw,
	}
}

// ListFull calls the List API server method.
func (c PublicClient) ListFull(patterns ...string) ([]payload.FullPayloadInfo, error) {
	var result api.EnvListResults

	args := api.EnvListArgs{
		Patterns: patterns,
	}
	if err := c.FacadeCall("List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	payloads := make([]payload.FullPayloadInfo, len(result.Results))
	for i, apiInfo := range result.Results {
		payload, err := api.API2Payload(apiInfo)
		if err != nil {
			// We should never see this happen; we control the input safely.
			return nil, errors.Trace(err)
		}
		payloads[i] = payload
	}
	return payloads, nil
}
