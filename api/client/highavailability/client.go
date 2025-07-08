// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the high availability service, used to manage controllers.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new HighAvailability client.
func NewClient(caller base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(caller, "HighAvailability", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// ControllerDetails holds details of a controller.
type ControllerDetails struct {
	ControllerID string
	APIEndpoints []string
}

func (c *Client) ControllerDetails(ctx context.Context) (map[string]ControllerDetails, error) {
	if c.BestAPIVersion() < 3 {
		return nil, errors.NotImplemented
	}
	var details params.ControllerDetailsResults
	err := c.facade.FacadeCall(ctx, "ControllerDetails", nil, &details)
	if err != nil {
		return nil, err
	}

	result := make(map[string]ControllerDetails)
	for _, r := range details.Results {
		if r.Error != nil {
			return nil, r.Error
		}
		result[r.ControllerId] = ControllerDetails{
			ControllerID: r.ControllerId,
			APIEndpoints: r.APIAddresses,
		}
	}
	return result, nil
}
