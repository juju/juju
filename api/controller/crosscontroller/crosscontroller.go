// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the CrossController API facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client-side CrossModelRelations facade.
func NewClient(caller base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(caller, "CrossController", options...)
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

// ControllerInfo contains the information about the controller that will be
// returned by the ControllerInfo method.
type ControllerInfo struct {
	Addrs  []string
	CACert string
}

// ControllerInfo returns the remote controller's API information.
func (c *Client) ControllerInfo() (*ControllerInfo, error) {
	var results params.ControllerAPIInfoResults
	if err := c.facade.FacadeCall(context.TODO(), "ControllerInfo", nil, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	info := results.Results[0]
	return &ControllerInfo{
		Addrs:  info.Addresses,
		CACert: info.CACert,
	}, nil
}

// WatchControllerInfo returns a watcher that is notified when the remote
// controller's API information changes.
func (c *Client) WatchControllerInfo() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	if err := c.facade.FacadeCall(context.TODO(), "WatchControllerInfo", nil, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}
