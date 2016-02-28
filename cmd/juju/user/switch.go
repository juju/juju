// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/modelcmd"
)

const switchUserCommandDoc = `
Switch to a different user in the controller.
`

type switchUserCommand struct {
	modelcmd.ControllerCommandBase
	User string
}

func NewSwitchUserCommand() cmd.Command {
	return modelcmd.WrapController(&switchUserCommand{})
}

// Info implements Command.Info.
func (c *switchUserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch-user",
		Args:    "<username>",
		Purpose: "switches to a different user in the controller",
		Doc:     switchUserCommandDoc,
	}
}

// Init implements Command.Init.
func (c *switchUserCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("you must specify a user to switch to")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}
	c.User = args[0]
	return nil
}

// Run implements Command.Run.
func (c *switchUserCommand) Run(ctx *cmd.Context) error {
	if !names.IsValidUser(c.User) {
		return errors.NotValidf("user %q", c.User)
	}
	store := c.ClientStore()
	controllerName := c.ControllerName()
	accountName := names.NewUserTag(c.User).Canonical()
	currentAccountName, err := store.CurrentAccount(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if accountName == currentAccountName {
		ctx.Infof("%s (no change)", accountName)
		return nil
	}
	if err := store.SetCurrentAccount(controllerName, accountName); err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("%s -> %s", currentAccountName, accountName)
	return nil
}
