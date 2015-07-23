// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/plugin"
	"gopkg.in/juju/charm.v5"
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

// FindPluginFn will find a plugin given its name.
type FindPluginFn func(string) (*plugin.Plugin, error)

// LaunchPluginFn will launch a plugin given a plugin and a process definition.
type LaunchPluginFn func(plugin.Plugin, charm.Process) (process.Details, error)

// NewProcLaunchCommand constructs a new ProcLaunchCommand.
func NewProcLaunchCommand(findPlugin FindPluginFn, launchPlugin LaunchPluginFn, ctx HookContext) (*ProcLaunchCommand, error) {

	base, err := newRegisteringCommand(ctx)
	if err != nil {
		return nil, err
	}

	c := &ProcLaunchCommand{
		registeringCommand: *base,
		findPlugin:         findPlugin,
		launchPlugin:       launchPlugin,
	}
	c.cmdInfo = LaunchCommandInfo
	c.handleArgs = c.init
	return c, nil
}

// ProcLaunchCommand implements the launch command for launching
// workflow processes.
type ProcLaunchCommand struct {
	registeringCommand

	findPlugin   FindPluginFn
	launchPlugin LaunchPluginFn
}

// Run implements cmd.Command.
func (c *ProcLaunchCommand) Run(ctx *cmd.Context) error {
	if err := c.registeringCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	info, err := c.findValidInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Move the plugin lookup over to a method in baseCommand.
	// TODO(ericsnow) Fix this to support Windows.
	envPath := ctx.Getenv("PATH") + ":" + os.Getenv("PATH")
	if err := os.Setenv("PATH", envPath); err != nil {
		return errors.Trace(err)
	}
	plugin, err := c.findPlugin(info.Type)
	if err != nil {
		return err
	}

	// The plugin is responsible for validating that the launch was
	// successful and returning an err if not. If err is not set, we
	// assume success, and that the procDetails are for informational
	// purposes.
	procDetails, err := c.launchPlugin(*plugin, info.Process)
	if err != nil {
		return errors.Trace(err)
	}
	c.Details = procDetails

	if err := c.register(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
