// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// LaunchCommandInfo is the info for the workload-launch command.
var LaunchCommandInfo = cmdInfo{
	Name:    "workload-launch",
	Summary: "launch a workload",
	Doc: `
"workload-launch" is used to launch a workload.

The workload name must correspond to one of the workloads defined in
the charm's workloads.yaml.`,
}

// NewWorkloadLaunchCommand constructs a new WorkloadLaunchCommand.
func NewWorkloadLaunchCommand(ctx HookContext) (*WorkloadLaunchCommand, error) {

	base, err := newTrackingCommand(ctx)
	if err != nil {
		return nil, err
	}

	c := &WorkloadLaunchCommand{
		trackingCommand: *base,
	}
	c.cmdInfo = LaunchCommandInfo
	c.handleArgs = c.init
	return c, nil
}

// WorkloadLaunchCommand implements the launch command for launching
// workloads.
type WorkloadLaunchCommand struct {
	trackingCommand
}

// Run implements cmd.Command.
func (c *WorkloadLaunchCommand) Run(ctx *cmd.Context) error {
	logger.Tracef("running %s command", LaunchCommandInfo.Name)
	if err := c.trackingCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	info, err := c.findValidInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Move OS env info into the plugin.
	// TODO(ericsnow) Fix this to support Windows.
	envPath := ctx.Getenv("PATH") + ":" + os.Getenv("PATH")
	if err := os.Setenv("PATH", envPath); err != nil {
		return errors.Trace(err)
	}

	plugin, err := c.compCtx.Plugin(info)
	if err != nil {
		return errors.Trace(err)
	}

	// The plugin is responsible for validating that the launch was
	// successful and returning an err if not. If err is not set, we
	// assume success, and that the workloadDetails are for informational
	// purposes.
	workloadDetails, err := plugin.Launch(info.Workload)
	if err != nil {
		return errors.Trace(err)
	}
	c.Details = workloadDetails

	// TODO(ericsnow) Register the workload even if it fails?
	//  (e.g. set a status with a "failed" state)

	if err := c.track(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
