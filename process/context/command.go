// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	ctx     jujuc.Context
	compCtx jujuc.ContextComponent

	// Name is the name of the process in charm metadata.
	Name string
	// info is the process info for the named workload process.
	info *process.Info
}

func newCommand(ctx jujuc.Context) baseCommand {
	compCtx, err := contextComponent(ctx)
	if err != nil {
		// The component wasn't registered properly.
		panic(err)
	}
	return baseCommand{
		ctx:     ctx,
		compCtx: compCtx,
	}
}

// Init implements cmd.Command.
func (c *baseCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("missing process name")
	}
	return errors.Trace(c.init(args[0]))
}

func (c *baseCommand) init(name string) error {
	var pInfo process.Info
	if err := c.compCtx.Get(name, pInfo); err != nil {
		return errors.Trace(err)
	}
	c.info = &pInfo
	c.Name = name
	return nil
}

// registeringCommand is the base for commands that register a process
// that has been launched.
type registeringCommand struct {
	baseCommand

	// Id is the unique ID for the launched process.
	Id string
	// Details is the launch details returned from the process plugin.
	Details process.LaunchDetails
	// Space is the network space.
	Space string
	// Env is the environment variables for inside the process environment.
	Env map[string]string

	env []string
}

func newRegisteringCommand(ctx jujuc.Context) registeringCommand {
	return registeringCommand{
		baseCommand: newCommand(ctx),
	}
}

// SetFlags implements cmd.Command.
func (c *registeringCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Space, "space", "", "network space")
	f.Var(cmd.NewAppendStringsValue(&c.env), "env", "environment variables")
}

func (c *registeringCommand) init(name string) error {
	if err := c.baseCommand.init(name); err != nil {
		return errors.Trace(err)
	}
	if err := c.checkSpace(); err != nil {
		return errors.Trace(err)
	}
	env := c.parseEnv()

	c.Env = env
	return nil
}

// checkSpace ensures that the requested network space is available
// to the hook.
func (c *registeringCommand) checkSpace() error {
	// TODO(ericsnow) Finish!
	return errors.Errorf("not finished")
}

// parseEnv parses the provided env vars and merges them with the ones
// in the charm metadata.
func (c *registeringCommand) parseEnv() map[string]string {
	envVars := make(map[string]string, len(c.info.Process.Env)+len(c.env))
	for k, v := range c.info.Process.Env {
		envVars[k] = v
	}
	for k, v := range c.env {
		envVars[k] = v
	}
	return envVars
}

// register updates the hook context with the information for the
// registered workload process. An error is returned if the process
// was already registered.
func (c *registeringCommand) register() error {
	if c.info.Status != process.StatusPending {
		return errors.Errorf("already registered")
	}
	c.info.Space = c.Space
	c.info.Details = c.Details
	c.info.EnvVars = c.Env
	if err := c.compCtx.Set(c.Name, c.info); err != nil {
		return errors.Trace(err)
	}
	return nil
}
