// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/v2/rpc/params"
)

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) (EnqueuedActions, error) {
	args := params.RunParams{Commands: commands, Timeout: timeout}

	if c.BestAPIVersion() > 6 {
		var results params.EnqueuedActionsV2
		err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
		if err != nil {
			return EnqueuedActions{}, errors.Trace(err)
		}
		return unmarshallEnqueuedActionsV2(results)
	}

	var results params.ActionResults
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

	if c.BestAPIVersion() > 6 {
		var results params.EnqueuedActionsV2
		err := c.facade.FacadeCall("Run", args, &results)
		if err != nil {
			return EnqueuedActions{}, errors.Trace(err)
		}
		return unmarshallEnqueuedActionsV2(results)
	}

	var results params.ActionResults
	err := c.facade.FacadeCall("Run", args, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedRunActions(results.Results)
}
