// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the bundle API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the bundle api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Bundle")
	return &Client{
		ClientFacade: frontend,
		facade:       backend}
}

// ExportBundle exports the current model configuration.
func (c *Client) ExportBundle(includeDefaults bool) (string, error) {
	var result params.StringResult
	arg := params.ExportBundleParams{
		IncludeCharmDefaults: includeDefaults,
	}
	if err := c.facade.FacadeCall("ExportBundle", arg, &result); err != nil {
		return "", errors.Trace(err)
	}

	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}

	return result.Result, nil
}
