// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
)

type rawAPI interface {
	facadeCaller
	Close() error
}

// PublicClient provides methods for interacting with Juju's public
// RPC API, relative to payloads.
type PublicClient struct {
	facadeCaller
	closeFunc func() error
}

// NewPublicClient builds a new payload API client.
func NewPublicClient(raw rawAPI) PublicClient {
	return PublicClient{
		facadeCaller: raw,
		closeFunc:    raw.Close,
	}
}

// Close closes the client.
func (c PublicClient) Close() error {
	if err := c.closeFunc(); err != nil {
		return errors.Trace(err)
	}
	return nil
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
