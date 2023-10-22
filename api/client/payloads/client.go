// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

// Client provides methods for interacting with Juju's public
// RPC API, relative to payloads.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new Client for the given raw API caller.
func NewClient(apiCaller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(apiCaller, "Payloads")

	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

// ListFull calls the List API server method.
func (c Client) ListFull(patterns ...string) ([]payloads.FullPayloadInfo, error) {
	var result params.PayloadListResults

	args := params.PayloadListArgs{
		Patterns: patterns,
	}
	if err := c.facade.FacadeCall(context.TODO(), "List", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	payloads := make([]payloads.FullPayloadInfo, len(result.Results))
	for i, apiInfo := range result.Results {
		payload, err := API2Payload(apiInfo)
		if err != nil {
			// We should never see this happen; we control the input safely.
			return nil, errors.Trace(err)
		}
		payloads[i] = payload
	}
	return payloads, nil
}
