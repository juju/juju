// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The metricsmanager package contains implementation for an api facade to
// access metrics functions within state
package metricsmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the metrics manager api
type Client struct {
	base.ClientFacade
	st     *api.State
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the metricsmanager api
func NewClient(st *api.State) *Client {
	frontend, backend := base.NewClientFacade(st, "MetricsManager")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// CleanupOldMetrics looks for metrics that are 24 hours old (or older)
// and have been sent. Any metrics it finds are deleted.
func (c *Client) CleanupOldMetrics() error {
	p := params.Entities{Entities: []params.Entity{
		{c.st.EnvironTag()},
	}}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("CleanupOldMetrics", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
