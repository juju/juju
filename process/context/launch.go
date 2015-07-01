// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/plugin"
	"gopkg.in/juju/charm.v5"
)

const launchDoc = `
"launch" is used to launch a workload process.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.`

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

	return &ProcLaunchCommand{
		registeringCommand: *base,
		findPlugin:         findPlugin,
		launchPlugin:       launchPlugin,
	}, nil
}

// ProcLaunchCommand implements the launch command for launching
// workflow processes.
type ProcLaunchCommand struct {
	registeringCommand

	findPlugin   FindPluginFn
	launchPlugin LaunchPluginFn
}

// Info implements cmd.Command.
func (c *ProcLaunchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "launch",
		Args:    "<name>",
		Purpose: "launch a workload process",
		Doc:     launchDoc,
	}
}

// Init implements cmd.Command.
func (c *ProcLaunchCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("expected %s, got %v", c.Info().Args, args)
	}

	return c.init(args[0])
}

func (c *ProcLaunchCommand) init(name string) error {
	if err := c.registeringCommand.init(name); err != nil {
		return err
	}
	return nil
}

// Run implements cmd.Command.
func (c *ProcLaunchCommand) Run(ctx *cmd.Context) error {

	plugin, err := c.findPlugin(c.Name)
	if err != nil {
		return err
	}

	// The plugin is responsible for validating that the launch was
	// successful and returning an err if not. If err is not set, we
	// assume success, and that the procDetails are for informational
	// purposes.
	procDetails, err := c.launchPlugin(*plugin, c.baseCommand.info.Process)
	if err != nil {
		return err
	}
	c.Details = procDetails

	if err := c.register(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
