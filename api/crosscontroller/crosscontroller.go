// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crosscontroller

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.api.crosscontroller")

// Client provides access to the CrossController API facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client-side CrossModelRelations facade.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "CrossController")
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
	if err := c.facade.FacadeCall("ControllerInfo", nil, &results); err != nil {
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
	if err := c.facade.FacadeCall("WatchControllerInfo", nil, &results); err != nil {
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
