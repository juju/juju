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

var registerDoc = `
register doc ....
`[1:]

func init() {
	jujuc.RegisterCommand("register", NewRegisterCommand)
}

// RegisterCommand implements the register command.
type RegisterCommand struct {
	cmd.CommandBase
	ctx jujuc.Context
	out cmd.Output

	// Name is the name of the process in charm metadata.
	Name string
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

// NewRegisterCommand returns a new RegisterCommand.
func NewRegisterCommand(ctx jujuc.Context) cmd.Command {
	return &RegisterCommand{ctx: ctx}
}

// Info implements cmd.Command.Info.
func (c *RegisterCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "register",
		Args:    "<name> <id> [<details>]",
		Purpose: "register a workload process",
		Doc:     registerDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *RegisterCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Space, "space", "", "network space")
	f.Var(cmd.NewAppendStringsValue(&c.env), "env", "environment variables")
}

// Init implements cmd.Command.Init.
func (c *RegisterCommand) Init(args []string) error {
	var details process.LaunchDetails

	switch len(args) {
	case 0, 1:
		return errors.Errorf("expected at least 2 args, got: %v", args)
	case 2:
		// Nothing to do.
	case 3:
		var err error
		if details, err = process.ParseDetails(args[2]); err != nil {
			return errors.Trace(err)
		}
	default:
		return errors.Errorf("expected at most 3 args, got: %v", args)
	}

	env, err := parseEnv(c.env)
	if err != nil {
		return errors.Trace(err)
	}

	c.Name = args[0]
	c.Id = args[1]
	c.Details = details
	c.Env = env

	return nil
}

func parseEnv(e []string) (map[string]string, error) {
	return nil, nil
}

// Run implements cmd.Command.Run.
func (c *RegisterCommand) Run(ctx *cmd.Context) error {
	compCtx, err := c.ctx.Component("process")
	if err != nil {
		return errors.Trace(err)
	}

	var pInfo *process.Info
	if err := compCtx.Get(c.Name, pInfo); err != nil {
		return errors.Trace(err)
	}

	if pInfo.Status != process.StatusPending {
		return errors.Errorf("already registered")
	}

	pInfo.Space = c.Space
	pInfo.Details = c.Details
	pInfo.EnvVars = c.Env
	// TODO(wwitzel3) should charmer have direct access to pInfo.Status?

	if err := compCtx.Set(c.Name, pInfo); err != nil {
		return errors.Trace(err)
	}

	return nil
}
