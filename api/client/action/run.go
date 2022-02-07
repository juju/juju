// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) (EnqueuedActions, error) {
	var results params.ActionResults
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedRunActions(results.Results)
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, applications and units slices.
func (c *Client) Run(run RunParams) (EnqueuedActions, error) {
	args := params.RunParams{
		Commands:        run.Commands,
		Timeout:         run.Timeout,
		Machines:        run.Machines,
		Applications:    run.Applications,
		Units:           run.Units,
		WorkloadContext: run.WorkloadContext,
	}
	var results params.ActionResults
	err := c.facade.FacadeCall("Run", args, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedRunActions(results.Results)
}
