// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/state/api/usermanager"
)

type UserCommand struct {
	*cmd.SuperCommand
}

type UserCommandBase struct {
	envcmd.EnvCommandBase
}

// NewUserManagerClient returns a usermanager client for the root api endpoint
// that the environment command returns.
func (c *UserCommandBase) NewUserManagerClient() (*usermanager.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return usermanager.NewClient(root), nil
}

const userCommandDoc = `
"juju user" is used to manage the user accounts and access control in
the Juju environment.
`

const userCommandPurpose = "manage user accounts and access control"

func NewUserCommand() cmd.Command {
	usercmd := &UserCommand{
		SuperCommand: cmd.NewSuperCommand(cmd.SuperCommandParams{
			Name:        "user",
			Doc:         userCommandDoc,
			UsagePrefix: "juju",
			Purpose:     userCommandPurpose,
		}),
	}
	// Define each subcommand in a separate "user_FOO.go" source file
	// (with tests in user_FOO_test.go) and wire in here.
	usercmd.Register(envcmd.Wrap(&UserAddCommand{}))
	usercmd.Register(envcmd.Wrap(&UserChangePasswordCommand{}))
	return usercmd
}
