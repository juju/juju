// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"code.google.com/p/go.crypto/ssh/terminal"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

const addUserDoc = `
Add users to an existing environment
The user information is stored within an existing environment, and will be lost
when the environent is destroyed.

Examples:
  juju add-user foobar mypass       (Add user foobar with password mypass)
  juju add-user foobar              (Add user foobar. A prompt will request the password)
`

type AddUserCommand struct {
	envcmd.EnvCommandBase
	User     string
	Password string
}

func (c *AddUserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-user",
		Args:    "<username> <password>",
		Purpose: "adds a user",
		Doc:     addUserDoc,
	}
}

func (c *AddUserCommand) Init(args []string) error {
	err := c.EnsureEnvNameSet()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User = args[0]

	if len(args) == 1 {
		fmt.Print("password: ")
		pass, err := terminal.ReadPassword(0)
		if err != nil {
			return fmt.Errorf("Failed to read password %v", err)
		}
		c.Password = string(pass)
	} else {
		c.Password = args[1]
	}
	return nil
}

func (c *AddUserCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewUserManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.AddUser(c.User, c.Password)
	return err
}
