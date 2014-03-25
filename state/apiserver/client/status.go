// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

// FullStatus gives the information needed for juju status over the api
func (c *Client) FullStatus(args params.StatusParams) (api.Status, error) {
	conn, err := juju.NewConnFromState(c.api.state)
	if err != nil {
		return api.Status{}, err
	}

	status, err := statecmd.Status(conn, args.Patterns)
	return *status, err
}

// Status is a stub version of FullStatus that was introduced in 1.16
func (c *Client) Status() (api.LegacyStatus, error) {
	var legacyStatus api.LegacyStatus
	status, err := c.FullStatus(params.StatusParams{})
	if err != nil {
		return legacyStatus, err
	}

	legacyStatus.Machines = make(map[string]api.LegacyMachineStatus)
	for machineName, machineStatus := range status.Machines {
		legacyStatus.Machines[machineName] = api.LegacyMachineStatus{
			InstanceId: string(machineStatus.InstanceId),
		}
	}
	return legacyStatus, nil
}
