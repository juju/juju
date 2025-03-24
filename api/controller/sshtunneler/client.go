// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// Client holds the SSH server client for it's respective worker.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns an SSH server facade client.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "SSHTunneler")
	return &Client{
		facade: facadeCaller,
	}
}

// ControllerAddresses returns a list of addresses for the specified controller machine.
func (c *Client) ControllerAddresses(machineTag names.MachineTag) (network.SpaceAddresses, error) {
	machine := params.Entity{Tag: machineTag.String()}
	var result params.StringsResult
	if err := c.facade.FacadeCall("ControllerAddresses", machine, &result); err != nil {
		return network.SpaceAddresses{}, err
	}
	if result.Error != nil {
		return network.SpaceAddresses{}, result.Error
	}
	return network.NewSpaceAddresses(result.Result...), nil
}

// InsertSSHConnRequest inserts a new SSH connection request into the state.
func (c *Client) InsertSSHConnRequest(arg state.SSHConnRequestArg) error {
	req := params.SSHConnRequestArg(arg)
	var result params.ErrorResult
	if err := c.facade.FacadeCall("InsertSSHConnRequest", req, &result); err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
