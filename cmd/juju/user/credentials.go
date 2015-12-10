// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

const userCredentialsDoc = `
Writes out the current user and credentials to a file that can be used
with 'juju controller login' to allow the user to access the same environments
as the same user from another machine.

Examples:

    $ juju get-user-credentials --output staging.creds

    # copy the staging.creds file to another machine

    $ juju login staging --server staging.creds --keep-password


See Also:
    juju help login
`

func NewCredentialsCommand() cmd.Command {
	return envcmd.WrapController(&credentialsCommand{})
}

// credentialsCommand changes the password for a user.
type credentialsCommand struct {
	envcmd.ControllerCommandBase
	OutPath string
}

// Info implements Command.Info.
func (c *credentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-user-credentials",
		Purpose: "save the credentials and server details to a file",
		Doc:     userCredentialsDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *credentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.OutPath, "o", "", "specifies the path of the generated file")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Run implements Command.Run.
func (c *credentialsCommand) Run(ctx *cmd.Context) error {
	creds, err := c.ConnectionCredentials()
	if err != nil {
		return errors.Trace(err)
	}

	filename := c.OutPath
	if filename == "" {
		// The reason for the dance though the newUserTag
		// is to strip off the optional provider.
		//   user -> user
		//   user@local -> user
		//   user@remote -> user
		name := names.NewUserTag(creds.User).Name()
		filename = fmt.Sprintf("%s.server", name)
	}
	return writeServerFile(c, ctx, creds.User, creds.Password, filename)
}
