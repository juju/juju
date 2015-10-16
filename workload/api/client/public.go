// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
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

// List calls the List API server method.
func (c PublicClient) List(patterns ...string) ([]workload.Payload, error) {
	var result api.EnvListResults

	args := api.EnvListArgs{
		Patterns: patterns,
	}
	if err := c.FacadeCall("List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	payloads := make([]workload.Payload, len(result.Results))
	for i, apiInfo := range result.Results {
		payload := api.API2Payload(apiInfo)
		payloads[i] = payload
	}
	return payloads, nil
}
