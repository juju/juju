// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// TrackCommandInfo is the info for the workload-track command.
var TrackCommandInfo = cmdInfo{
	Name:      "workload-track",
	ExtraArgs: []string{"workload-details"},
	Summary:   "track a workload",
	Doc: `
"workload-track" is used while a hook is running to let Juju know
that a workload has been started. The information
used to start the workload must be provided when "track" is run.

The workload name must correspond to one of the workloads defined in
the charm's workloads.yaml.
`,
}

// TODO(ericsnow) Also support setting the juju-level status?

// WorkloadTrackCommand implements the track command.
type WorkloadTrackCommand struct {
	trackingCommand
}

// NewWorkloadTrackCommand returns a new WorkloadTrackCommand.
func NewWorkloadTrackCommand(ctx HookContext) (*WorkloadTrackCommand, error) {
	base, err := newTrackingCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := &WorkloadTrackCommand{
		trackingCommand: *base,
	}
	c.cmdInfo = TrackCommandInfo
	c.handleArgs = c.init
	return c, nil
}

func (c *WorkloadTrackCommand) init(args map[string]string) error {
	if err := c.trackingCommand.init(args); err != nil {
		return errors.Trace(err)
	}

	detailsStr := args["workload-details"]
	details, err := workload.UnmarshalDetails([]byte(detailsStr))
	if err != nil {
		return errors.Trace(err)
	}
	c.Details = details

	return nil
}

// Run implements cmd.Command.
func (c *WorkloadTrackCommand) Run(ctx *cmd.Context) error {
	if err := c.trackingCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	info, err := c.findValidInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// Ensure that the plugin is correct.
	_, err = c.compCtx.Plugin(info, ctx.Getenv("PATH"))
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(wwitzel3) should charmer have direct access to pInfo.Status?
	if err := c.track(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
