// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

const removeUserDoc = `
`

type RemoveUserCommand struct {
	envcmd.EnvCommandBase
	User string
}

func (c *RemoveUserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-user",
		Args:    "<username>",
		Purpose: "removes a user",
		Doc:     removeUserDoc,
	}
}

func (c *RemoveUserCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	c.User = args[0]

	return cmd.CheckEmpty(args[1:])
}

func (c *RemoveUserCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewUserManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.RemoveUser(c.User)
	return err
}
