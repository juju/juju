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
		return "", errors.Errorf("this controller version does not support bundle export feature.")
	}

	if err := c.facade.FacadeCall("ExportBundle", nil, &result); err != nil {
		return "", errors.Trace(err)
	}

	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}

	return result.Result, nil
}
