// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(ctx context.Context, commands string, timeout time.Duration) (EnqueuedActions, error) {
	var results params.EnqueuedActions
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.facade.FacadeCall(ctx, "RunOnAllMachines", args, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedActions(results)
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, applications and units slices.
func (c *Client) Run(ctx context.Context, run RunParams) (EnqueuedActions, error) {
	args := params.RunParams{
		Commands:     run.Commands,
		Timeout:      run.Timeout,
		Machines:     run.Machines,
		Applications: run.Applications,
		Units:        run.Units,
	}
	var results params.EnqueuedActions
	err := c.facade.FacadeCall(ctx, "Run", args, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedActions(results)
}
