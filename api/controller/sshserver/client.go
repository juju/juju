// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client holds the SSH server client for it's respective worker.
type Client struct {
	facade base.FacadeCaller
	*common.ControllerConfigAPI
}

// NewClient returns an SSH server facade client.
func NewClient(caller base.APICaller) (*Client, error) {
	_, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "SSHServer")
	return &Client{
		facade:              facadeCaller,
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}, nil
}

// WatchControllerConfig provides a watcher for changes on controller config.
func (c *Client) WatchControllerConfig() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall("WatchControllerConfig", nil, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
