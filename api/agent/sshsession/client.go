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
	var result params.StringsWatchResult
	sshConnRequestWatchArg := params.SSHConnRequestWatchArg{
		MachineId: machineId,
	}

	if err := c.facade.FacadeCall("WatchSSHConnRequest", sshConnRequestWatchArg, &result); err != nil {
		return nil, err
	}

	if err := result.Error; err != nil {
		return nil, errors.Trace(err)
	}

	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// GetSSHConnRequest returns a ssh connection request by its connection request ID.
func (c *Client) GetSSHConnRequest(requestId string) (params.SSHConnRequest, error) {
	var results params.SSHConnRequestResult
	if requestId == "" {
		return results.SSHConnRequest, errors.New("connection request id cannot be empty")
	}
	arg := params.SSHConnRequestGetArg{
		RequestId: requestId,
	}

	if err := c.facade.FacadeCall("GetSSHConnRequest", arg, &results); err != nil {
		return results.SSHConnRequest, errors.Trace(err)
	}

	if err := results.Error; err != nil {
		return results.SSHConnRequest, errors.Trace(err)
	}

	return results.SSHConnRequest, nil
}
