// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/process"
)

// RegisterCommandInfo is the info for the proc-register command.
var RegisterCommandInfo = cmdInfo{
	Name:      "process-register",
	ExtraArgs: []string{"proc-details"},
	Summary:   "register a workload process",
	Doc: `
"process-register" is used while a hook is running to let Juju know
that a workload process has been manually started. The information
used to start the process must be provided when "register" is run.

The process name must correspond to one of the processes defined in
the charm's metadata.yaml.
`,
}

// TODO(ericsnow) Also support setting the juju-level status?

// ProcRegistrationCommand implements the register command.
type ProcRegistrationCommand struct {
	registeringCommand
}

// NewProcRegistrationCommand returns a new ProcRegistrationCommand.
func NewProcRegistrationCommand(ctx HookContext) (*ProcRegistrationCommand, error) {
	base, err := newRegisteringCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := &ProcRegistrationCommand{
		registeringCommand: *base,
	}
	c.cmdInfo = RegisterCommandInfo
	c.handleArgs = c.init
	return c, nil
}

func (c *ProcRegistrationCommand) init(args map[string]string) error {
	if err := c.registeringCommand.init(args); err != nil {
		return errors.Trace(err)
	}

	detailsStr := args["proc-details"]
	details, err := process.UnmarshalDetails([]byte(detailsStr))
	if err != nil {
		return errors.Trace(err)
	}
	c.Details = details

	return nil
}

// Run implements cmd.Command.
func (c *ProcRegistrationCommand) Run(ctx *cmd.Context) error {
	if err := c.registeringCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	// TODO(wwitzel3) should charmer have direct access to pInfo.Status?
	if err := c.register(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}
