// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

// StatusSetCommand implements the status-set command.
type StatusSetCommand struct {
	cmd.CommandBase
	ctx     Context
	status  string
	message string
	service bool
}

// NewStatusSetCommand makes a jujuc status-set command.
func NewStatusSetCommand(ctx Context) cmd.Command {
	return &StatusSetCommand{ctx: ctx}
}

func (c *StatusSetCommand) Info() *cmd.Info {
	doc := `
Sets the workload status of the charm. Message is optional.
The "last updated" attribute of the status is set, even if the
status and message are the same as what's already set.
`
	return &cmd.Info{
		Name:    "status-set",
		Args:    "<maintenance | blocked | waiting | active> [message]",
		Purpose: "set status information",
		Doc:     doc,
	}
}

var validStatus = []params.Status{
	params.StatusMaintenance,
	params.StatusBlocked,
	params.StatusWaiting,
	params.StatusActive,
}

func (c *StatusSetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.service, "service", false, "set this status for the service to which the unit belongs if the unit is the leader")
}

func (c *StatusSetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("invalid args, require <status> [message]")
	}
	valid := false
	for _, s := range validStatus {
		if string(s) == args[0] {
			valid = true
			break
		}
	}
	if !valid {
		return errors.Errorf("invalid status %q, expected one of %v", args[0], validStatus)
	}
	c.status = args[0]
	if len(args) > 1 {
		c.message = args[1]
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

func (c *StatusSetCommand) Run(ctx *cmd.Context) error {
	statusInfo := StatusInfo{
		Status: c.status,
		Info:   c.message,
	}
	if c.service {
		return c.ctx.SetServiceStatus(statusInfo)
	}
	return c.ctx.SetUnitStatus(statusInfo)

}
