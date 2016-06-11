// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var deleteUsageSummary = `
Deletes a Juju user from a controller.`[1:]

// TODO(redir): Get updated summary, details, and flag help copy.
// TODO(redir): Get updated copy for add-user as that may need updates, too.
// TODO(redir): Get confirmation on the controller bits.
var deleteUsageDetails = `
This deletes a user permanently. Deletion can be confirmed with
the -y --yes flag. 

By default, the controller is the current controller.

Examples:
    juju delete-user bob
	juju delete-user bob -y

See also: 
    unregister
    revoke
    show-user
    list-users
    switch-user
    disable-user
    enable-user
    change-user-password`[1:]

// DeleteUserAPI defines the usermanager API methods that the delete command
// uses.
type DeleteUserAPI interface {
	DeleteUser(username string) error
	Close() error
}

func NewDeleteCommand() cmd.Command {
	return modelcmd.WrapController(&deleteCommand{})
}

// deleteCommand deletes a user from a Juju controller.
type deleteCommand struct {
	modelcmd.ControllerCommandBase
	api           DeleteUserAPI
	UserName      string
	ConfirmDelete bool
}

func (c *deleteCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.ConfirmDelete, "yes", false, "Confirm deletion of the user")
}

// Info implements Command.Info.
func (c *deleteCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete-user",
		Args:    "<user name>",
		Purpose: deleteUsageSummary,
		Doc:     deleteUsageDetails,
	}
}

// Init implements Command.Init.
func (c *deleteCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.UserName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *deleteCommand) Run(ctx *cmd.Context) error {
	api := c.api // This is for testing.

	if api == nil { // The real McCoy.
		var err error
		api, err = c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer api.Close()
	}

	err := api.DeleteUser(c.UserName)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	fmt.Fprintf(ctx.Stdout, "User %q removed\n", c.UserName)

	return nil
}
