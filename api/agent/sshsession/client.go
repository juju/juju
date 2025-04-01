// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the SSH Session API.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the SSH Session API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "SSHSession")
	return &Client{
		facade: facadeCaller,
	}
}

// WatchSSHConnRequest creates a watcher and returns its ID for watching changes.
func (c *Client) WatchSSHConnRequest(machineId string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	if err := c.facade.FacadeCall("WatchSSHConnRequest", machineId, &results); err != nil {
		return nil, err
	}

	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}

	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// GetSSHConnRequest returns a ssh connection request by its connection request ID.
func (c *Client) GetSSHConnRequest(arg string) (params.SSHConnRequest, error) {
	var results params.SSHConnRequestResult
	if arg == "" {
		return results.SSHConnRequest, errors.New("connection request id cannot be empty")
	}

	if err := c.facade.FacadeCall("GetSSHConnRequest", arg, &results); err != nil {
		return results.SSHConnRequest, errors.Trace(err)
	}

	if err := results.Error; err != nil {
		return results.SSHConnRequest, errors.Trace(err)
	}

	return results.SSHConnRequest, nil
}
