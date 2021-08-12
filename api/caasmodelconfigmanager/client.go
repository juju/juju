// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
)

// Client allows access to the CAAS model config manager API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Application Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASModelConfigManager")
	return &Client{
		facade: facadeCaller,
	}
}

// WatchControllerConfig returns a NotifyWatcher that notifies of
// changes to the controller config.
func (c *Client) WatchControllerConfig() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall("WatchControllerConfig", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ControllerConfig returns the current controller configuration.
func (c *Client) ControllerConfig() (controller.Config, error) {
	var result params.ControllerConfigResult
	err := c.facade.FacadeCall("ControllerConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	return controller.Config(result.Config), nil
}
