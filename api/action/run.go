// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/juju/apiserver/params"
)

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) (params.EnqueuedActions, error) {
	var results params.EnqueuedActions
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
	return results, err
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, applications and units slices.
func (c *Client) Run(run params.RunParams) (params.EnqueuedActions, error) {
	var results params.EnqueuedActions
	err := c.facade.FacadeCall("Run", run, &results)
	return results, err
}
