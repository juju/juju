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

// ListFull calls the List API server method.
func (c PublicClient) ListFull(patterns ...string) ([]workload.FullPayloadInfo, error) {
	var result api.EnvListResults

	args := api.EnvListArgs{
		Patterns: patterns,
	}
	if err := c.FacadeCall("List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	payloads := make([]workload.FullPayloadInfo, len(result.Results))
	for i, apiInfo := range result.Results {
		// We ignore the error since we control the input safely.
		payload, _ := api.API2Payload(apiInfo)
		payloads[i] = payload
	}
	return payloads, nil
}
