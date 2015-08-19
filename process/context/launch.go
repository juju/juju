// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// LaunchCommandInfo is the info for the proc-launch command.
var LaunchCommandInfo = cmdInfo{
	Name:    "process-launch",
	Summary: "launch a workload process",
	Doc: `
"process-launch" is used to launch a workload process.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.`,
}

// NewProcLaunchCommand constructs a new ProcLaunchCommand.
func NewProcLaunchCommand(ctx HookContext) (*ProcLaunchCommand, error) {

	base, err := newRegisteringCommand(ctx)
	if err != nil {
		return nil, err
	}

	c := &ProcLaunchCommand{
		registeringCommand: *base,
	}
	c.cmdInfo = LaunchCommandInfo
	c.handleArgs = c.init
	return c, nil
}

// ProcLaunchCommand implements the launch command for launching
// workflow processes.
type ProcLaunchCommand struct {
	registeringCommand
}

// Run implements cmd.Command.
func (c *ProcLaunchCommand) Run(ctx *cmd.Context) error {
	logger.Tracef("running %s command", LaunchCommandInfo.Name)
	if err := c.registeringCommand.Run(ctx); err != nil {
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
	// assume success, and that the procDetails are for informational
	// purposes.
	procDetails, err := plugin.Launch(info.Process)
	if err != nil {
		return errors.Trace(err)
	}
	c.Details = procDetails

	// TODO(ericsnow) Register the proc even if it fails?
	//  (e.g. set a status with a "failed" state)

	if err := c.register(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
