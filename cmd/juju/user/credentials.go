// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"
)

const userCredentialsDoc = `
Writes out the current user and credentials to a file that can be used
with 'juju system login' to allow the user to access the same environments
as the same user from another machine.

Examples:

    $ juju user credentials --output staging.creds

    # copy the staging.creds file to another machine

    $ juju system login staging --server staging.creds --keep-password


See Also:
    juju system login
`

// CredentialsCommand changes the password for a user.
type CredentialsCommand struct {
	UserCommandBase
	OutPath string
}

// Info implements Command.Info.
func (c *CredentialsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "credentials",
		Purpose: "save the credentials and server details to a file",
		Doc:     userCredentialsDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *CredentialsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.OutPath, "o", "", "specifies the path of the generated file")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Run implements Command.Run.
func (c *CredentialsCommand) Run(ctx *cmd.Context) error {
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
