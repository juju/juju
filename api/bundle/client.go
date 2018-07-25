// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle provides access to the bundle api facade.
// This facade contains api calls that are specific to bundles.

package bundle

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.bundle")

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
func (c *Client) ExportBundle() (string, error) {
	var result params.StringResult
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return "", errors.Errorf("command not supported on v%d", bestVer)
	}

	if err := c.facade.FacadeCall("ExportBundle", nil, &result); err != nil {
		errors.Annotate(result.Error, "export failed")
		return "", errors.Trace(err)
	}

	if len(result.Result) == 0 {
		return "", errors.Annotate(result.Error, "export failed")
	}

	return result.Result, nil
}
