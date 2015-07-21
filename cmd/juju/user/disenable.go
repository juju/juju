// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/block"
)

const disableUserDoc = `
Disabling a user stops that user from being able to log in. The user still
exists and can be reenabled using the "juju enable" command.  If the user is
already disabled, this command succeeds silently.

Examples:
  juju user disable foobar

See Also:
  juju help user enable
`

const enableUserDoc = `
Enabling a user that is disabled allows that user to log in again. The user
still exists and can be reenabled using the "juju enable" command.  If the
user is already enabled, this command succeeds silently.

Examples:
  juju user enable foobar

See Also:
  juju help user disable
`

// DisenableUserBase common code for enable/disable user commands
type DisenableUserBase struct {
	UserCommandBase
	api  DisenableUserAPI
	User string
}

// DisableCommand disables users.
type DisableCommand struct {
	DisenableUserBase
}

// EnableCommand enables users.
type EnableCommand struct {
	DisenableUserBase
}

// Info implements Command.Info.
func (c *DisableCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "disable",
		Args:    "<username>",
		Purpose: "disable a user to stop the user logging in",
		Doc:     disableUserDoc,
	}
}

// Info implements Command.Info.
func (c *EnableCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "enable",
		Args:    "<username>",
		Purpose: "reenables a disabled user to allow the user to log in",
		Doc:     enableUserDoc,
	}
}

// Init implements Command.Init.
func (c *DisenableUserBase) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	// TODO(thumper): support multiple users in one command,
	// and also verify that the values are valid user names.
	c.User = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Username is here entirely for testing purposes to allow both the
// DisableCommand and EnableCommand to support a common interface that is able
// to ask for the command line supplied username.
func (c *DisenableUserBase) Username() string {
	return c.User
}

// DisenableUserAPI defines the API methods that the disable and enable
// commands use.
type DisenableUserAPI interface {
	EnableUser(username string) error
	DisableUser(username string) error
	Close() error
}

func (c *DisenableUserBase) getDisableUserAPI() (DisenableUserAPI, error) {
	return c.NewUserManagerAPIClient()
}

var getDisableUserAPI = (*DisenableUserBase).getDisableUserAPI

// Run implements Command.Run.
func (c *DisableCommand) Run(ctx *cmd.Context) error {
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
func (c *EnableCommand) Run(ctx *cmd.Context) error {
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
