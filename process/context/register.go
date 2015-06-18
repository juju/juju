// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

const registerDoc = `
"register" is used while a hook is running to let Juju know that
a workload process has been manually started. The information used
to start the process must be provided when "register" is run.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.
`

// ProcRegistrationCommand implements the register command.
type ProcRegistrationCommand struct {
	registeringCommand
}

// TODO(ericsnow) Refactor so that importing jujuc is not necessary.

// NewProcRegistrationCommand returns a new ProcRegistrationCommand.
func NewProcRegistrationCommand(ctx jujuc.Context) (*ProcRegistrationCommand, error) {
	base, err := newRegisteringCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProcRegistrationCommand{
		registeringCommand: *base,
	}, nil
}

// Info implements cmd.Command.
func (c *ProcRegistrationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "register",
		Args:    "<name> <id> [<details>]",
		Purpose: "register a workload process",
		Doc:     registerDoc,
	}
}

// Init implements cmd.Command.
func (c *ProcRegistrationCommand) Init(args []string) error {
	switch len(args) {
	case 0, 1:
		return errors.Errorf("expected at least 2 args, got: %v", args)
	case 2:
		return c.init(args[0], args[1], "")
	case 3:
		return c.init(args[0], args[1], args[2])
	default:
		return errors.Errorf("expected at most 3 args, got: %v", args)
	}
}

func (c *ProcRegistrationCommand) init(name, id, detailsStr string) error {
	if err := c.registeringCommand.init(name); err != nil {
		return errors.Trace(err)
	}

	if id == "" {
		return errors.Errorf("got empty id")
	}
	c.ID = id

	if detailsStr != "" {
		details, err := process.ParseDetails(detailsStr)
		if err != nil {
			return errors.Trace(err)
		}
		if details.ID != id {
			return errors.Errorf("ID in details (%s) does not match ID arg (%s)", details.ID, id)
		}
		c.Details = *details
	}

	return nil
}

// Run implements cmd.Command.
func (c *ProcRegistrationCommand) Run(ctx *cmd.Context) error {
	// TODO(wwitzel3) should charmer have direct access to pInfo.Status?
	if err := c.register(ctx, process.StatusActive); err != nil {
		return errors.Trace(err)
	}
	return nil
}
