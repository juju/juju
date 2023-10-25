// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

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

// Client allows access to the credential management API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the credential manager API.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "CredentialManager", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// InvalidateModelCredential invalidates cloud credential for the model that made a connection.
func (c *Client) InvalidateModelCredential(reason string) error {
	in := params.InvalidateCredentialArg{reason}
	var result params.ErrorResult
	err := c.facade.FacadeCall(context.TODO(), "InvalidateModelCredential", in, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}
