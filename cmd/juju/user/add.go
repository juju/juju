// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
)

const userAddCommandDoc = `
Add users to an existing environment.

The user information is stored within an existing environment, and will be
lost when the environent is destroyed.  A server file will be written out in
the current directory.  You can control the name and location of this file
using the --output option.

Examples:
    # Add user "foobar" with a strong random password is generated.
    juju user add foobar


See Also:
    juju help user change-password
`

// AddUserAPI defines the usermanager API methods that the add command uses.
type AddUserAPI interface {
	AddUser(username, displayName, password string) (names.UserTag, error)
	Close() error
}

// AddCommand adds new users into a Juju Server.
type AddCommand struct {
	UserCommandBase
	api         AddUserAPI
	User        string
	DisplayName string
	OutPath     string
}

// Info implements Command.Info.
func (c *AddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<username> [<display name>]",
		Purpose: "adds a user",
		Doc:     userAddCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *AddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.OutPath, "o", "", "specify the environment file for new user")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Init implements Command.Init.
func (c *AddCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User, args = args[0], args[1:]
	if len(args) > 0 {
		c.DisplayName, args = args[0], args[1:]
	}
	if c.OutPath == "" {
		c.OutPath = c.User + ".server"
	}
	return cmd.CheckEmpty(args)
}

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) error {
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	password, err := utils.RandomPassword()
	if err != nil {
		return errors.Annotate(err, "failed to generate random password")
	}
	randomPasswordNotify(password)

	if _, err := c.api.AddUser(c.User, c.DisplayName, password); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	displayName := c.User
	if c.DisplayName != "" {
		displayName = fmt.Sprintf("%s (%s)", c.DisplayName, c.User)
	}

	ctx.Infof("user %q added", displayName)

	return writeServerFile(c, ctx, c.User, password, c.OutPath)
}
