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
	st     api.Connection
	facade base.FacadeCaller
}

// MetricsManagerClient defines the methods on the metricsmanager API end point.
type MetricsManagerClient interface {
	CleanupOldMetrics() error
	SendMetrics() error
}

var _ MetricsManagerClient = (*Client)(nil)

// NewClient creates a new client for accessing the metricsmanager api
func NewClient(st api.Connection) *Client {
	frontend, backend := base.NewClientFacade(st, "MetricsManager")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// CleanupOldMetrics looks for metrics that are 24 hours old (or older)
// and have been sent. Any metrics it finds are deleted.
func (c *Client) CleanupOldMetrics() error {
	envTag, err := c.st.EnvironTag()
	if err != nil {
		return errors.Trace(err)
	}
	p := params.Entities{Entities: []params.Entity{
		{envTag.String()},
	}}
	results := new(params.ErrorResults)
	err = c.facade.FacadeCall("CleanupOldMetrics", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// SendMetrics will send any unsent metrics to the collection service.
func (c *Client) SendMetrics() error {
	envTag, err := c.st.EnvironTag()
	if err != nil {
		return errors.Trace(err)
	}
	p := params.Entities{Entities: []params.Entity{
		{envTag.String()},
	}}
	results := new(params.ErrorResults)
	err = c.facade.FacadeCall("SendMetrics", p, results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
