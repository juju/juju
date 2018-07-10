// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle provides access to the bundle api facade.
// This facade contains api calls that are specific to bundles.

package bundle

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.application")

// Client allows access to the application API end point.
type Client struct {
	base.ClientFacade
	st     base.APICallCloser
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the bundle api.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Bundle")
	return &Client{
		ClientFacade: frontend,
		st:           st,
		facade:       backend}
}

// ExportBundle exports the current model configuration.
func (c *Client) ExportBundle(tag names.ModelTag) (params.StringResult, error) {
	var results params.StringResult
	err := c.facade.FacadeCall("ExportBundle", tag, &results)
	if err != nil {
		return params.StringResult{}, errors.Trace(err)
	}
	if len(results.Result) != 0 {
		return params.StringResult{}, errors.Errorf("result obtained is incorrect.")
	}
	return results, nil
}
