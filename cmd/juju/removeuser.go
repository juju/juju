// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"github.com/juju/cmd"
)

const removeUserDoc = `
Remove users from an existing environment

Examples:
  juju remove-user foobar
`

type RemoveUserCommand struct {
	UserCommandBase
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
	client, err := c.NewUserManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return client.RemoveUser(c.User)
}
