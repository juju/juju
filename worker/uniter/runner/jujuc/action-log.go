// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

// ActionLogCommand implements the action-log command.
type ActionLogCommand struct {
	cmd.CommandBase
	ctx     Context
	name    string
	Message string
}

func NewActionLogCommand(ctx Context, name string) (cmd.Command, error) {
	return &ActionLogCommand{ctx: ctx, name: name}, nil
}

func (c *ActionLogCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    c.name,
		Args:    "<message>",
		Purpose: "record a progress message for the current action/function",
	})
}

func (c *ActionLogCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no message specified")
	}
	c.Message = strings.Join(args, " ")
	return nil
}

func (c *ActionLogCommand) Run(ctx *cmd.Context) error {
	return c.ctx.LogActionMessage(c.Message)
}
