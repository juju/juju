// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

const disableUserDoc = ` Disabling a user stops that user from being able to
log in. The user still exists and can be reenabled using the "juju enable"
command.  If the user is already disabled, this command succeeds silently.

Examples:
  juju user disable foobar

See Also:
  juju enable
`

const enableUserDoc = `

Enabling a user that is disabled allows that user to log in again. The user
still exists and can be reenabled using the "juju enable" command.  If the
user is already enabled, this command succeeds silently.

Examples:
  juju user enable foobar

See Also:
  juju disable
`

// DisableUserBase common code for enable/disable user commands
type DisableUserBase struct {
	UserCommandBase
	user string
}

// DisableCommand disables users.
type DisableCommand struct {
	DisableUserBase
}

// EnableCommand enables users.
type EnableCommand struct {
	DisableUserBase
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
func (c *DisableUserBase) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	c.user = args[0]
	return cmd.CheckEmpty(args[1:])
}

// DisenableUserAPI defines the API methods that the disable and enable
// commands use.
type DisenableUserAPI interface {
	EnableUser(username string) error
	DisableUser(username string) error
	Close() error
}

func (c *DisableUserBase) getDisableUserAPI() (DisenableUserAPI, error) {
	return c.NewUserManagerClient()
}

var getDisableUserAPI = (*DisableUserBase).getDisableUserAPI

// Info implements Command.Run.
func (c *DisableCommand) Run(ctx *cmd.Context) error {
	client, err := getDisableUserAPI(&c.DisableUserBase)
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.DisableUser(c.user)
	if err != nil {
		return err
	}
	ctx.Infof("User %q disabled", c.user)
	return nil
}

// Info implements Command.Run.
func (c *EnableCommand) Run(ctx *cmd.Context) error {
	client, err := getDisableUserAPI(&c.DisableUserBase)
	if err != nil {
		return err
	}
	defer client.Close()
	err = client.EnableUser(c.user)
	if err != nil {
		return err
	}
	ctx.Infof("User %q enabled", c.user)
	return nil
}
