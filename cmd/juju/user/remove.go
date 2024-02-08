// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

var removeUsageSummary = `
Deletes a Juju user from a controller.`[1:]

// TODO(redir): Get updated copy for add-user as that may need updates, too.
var removeUsageDetails = `
This removes a user permanently.

By default, the controller is the current controller.

`[1:]

const removeUsageExamples = `
    juju remove-user bob
    juju remove-user bob --yes
`

var removeUserMsg = `
WARNING! This command will permanently archive the user %q on the %q
controller. This action is irreversible and you WILL NOT be able to reuse
username %q.

If you wish to temporarily disable the user please use the`[1:] + " `juju disable-user`\n" + `command. See
` + " `juju help disable-user` " + `for more details.

Continue (y/N)? `

// RemoveUserAPI defines the usermanager API methods that the remove command
// uses.
type RemoveUserAPI interface {
	RemoveUser(username string) error
	Close() error
}

// NewRemoveCommand constructs a wrapped unexported removeCommand.
func NewRemoveCommand() cmd.Command {
	return modelcmd.WrapController(&removeCommand{})
}

// removeCommand deletes a user from a Juju controller.
type removeCommand struct {
	modelcmd.ControllerCommandBase
	api           RemoveUserAPI
	UserName      string
	ConfirmDelete bool
}

// SetFlags adds command specific flags and then returns the flagset.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.ConfirmDelete, "y", false, "Confirm deletion of the user")
	f.BoolVar(&c.ConfirmDelete, "yes", false, "")
}

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-user",
		Args:     "<user name>",
		Purpose:  removeUsageSummary,
		Doc:      removeUsageDetails,
		Examples: removeUsageExamples,
		SeeAlso: []string{
			"unregister",
			"revoke",
			"show-user",
			"users",
			"disable-user",
			"enable-user",
			"change-user-password",
		},
	})
}

// Init implements Command.Init.
func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no username supplied")
	}
	c.UserName = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	api := c.api // This is for testing.

	if api == nil { // The real McCoy.
		var err error
		api, err = c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer api.Close()
	}

	// Confirm deletion if the user didn't specify -y/--yes in the command.
	if !c.ConfirmDelete {
		if err := confirmDelete(ctx, controllerName, c.UserName); err != nil {
			return errors.Trace(err)
		}
	}

	if err := api.RemoveUser(c.UserName); err != nil {
		// This is very awful, but it makes the user experience crisper. At
		// least maybe more tenable until users and authn/z are overhauled.
		if e, ok := err.(*params.Error); ok {
			if e.Message == fmt.Sprintf("failed to delete user %q: user %q is permanently deleted", c.UserName, c.UserName) {
				e.Message = fmt.Sprintf("failed to delete user %q: the user has already been permanently deleted", c.UserName)
				err = e
			}
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	fmt.Fprintf(ctx.Stdout, "User %q removed\n", c.UserName)

	return nil
}

func confirmDelete(ctx *cmd.Context, controller, username string) error {
	// Get confirmation from the user that they want to continue
	fmt.Fprintf(ctx.Stdout, removeUserMsg, username, controller, username)

	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return errors.Annotate(err, "user deletion aborted")
	}
	answer := strings.ToLower(scanner.Text())
	if answer != "y" && answer != "yes" {
		return errors.New("user deletion aborted")
	}
	return nil
}
