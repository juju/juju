// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

const disableUserDoc = `
Disabling a user stops that user from being able to log in. The user still
exists and can be reenabled using the "juju enable-user" command.  If the user is
already disabled, this command succeeds silently.

Examples:
  juju disable-user foobar

See Also:
  juju help enable-user
`

const enableUserDoc = `
Enabling a user that is disabled allows that user to log in again. The user
still exists and can be reenabled using the "juju enable-user" command.  If the
user is already enabled, this command succeeds silently.

Examples:
  juju enable-user foobar

See Also:
  juju help disable-user
`

// disenableUserBase common code for enable/disable user commands
type disenableUserBase struct {
	modelcmd.ControllerCommandBase
	api  disenableUserAPI
	User string
}

func NewDisableCommand() cmd.Command {
	return modelcmd.WrapController(&disableCommand{})
}

// disableCommand disables users.
type disableCommand struct {
	disenableUserBase
}

func NewEnableCommand() cmd.Command {
	return modelcmd.WrapController(&enableCommand{})
}

// enableCommand enables users.
type enableCommand struct {
	disenableUserBase
}

// Info implements Command.Info.
func (c *disableCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "disable-user",
		Args:    "<username>",
		Purpose: "disable a user to stop the user logging in",
		Doc:     disableUserDoc,
	}
}

// Info implements Command.Info.
func (c *enableCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "enable-user",
		Args:    "<username>",
		Purpose: "reenables a disabled user to allow the user to log in",
		Doc:     enableUserDoc,
	}
}

// Init implements Command.Init.
func (c *disenableUserBase) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	// TODO(thumper): support multiple users in one command,
	// and also verify that the values are valid user names.
	c.User = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Username is here entirely for testing purposes to allow both the
// disableCommand and enableCommand to support a common interface that is able
// to ask for the command line supplied username.
func (c *disenableUserBase) Username() string {
	return c.User
}

// disenableUserAPI defines the API methods that the disable and enable
// commands use.
type disenableUserAPI interface {
	EnableUser(username string) error
	DisableUser(username string) error
	Close() error
}

func (c *disenableUserBase) getDisableUserAPI() (disenableUserAPI, error) {
	return c.NewUserManagerAPIClient()
}

var getDisableUserAPI = (*disenableUserBase).getDisableUserAPI

// Run implements Command.Run.
func (c *disableCommand) Run(ctx *cmd.Context) error {
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	if err := c.api.DisableUser(c.User); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("User %q disabled", c.User)
	return nil
}

// Run implements Command.Run.
func (c *enableCommand) Run(ctx *cmd.Context) error {
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	if err := c.api.EnableUser(c.User); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("User %q enabled", c.User)
	return nil
}
