// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

type RemoveuserCommand struct {
	cmd.EnvCommandBase
	Tag string
}

func (c *RemoveuserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-user",
		Args:    "<username>",
		Purpose: "removes a user",
	}
}

func (c *RemoveuserCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	c.Tag = args[0]

	return cmd.CheckEmpty(args[1:])
}

func (c *RemoveuserCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewUserManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.RemoveUser(c.Tag)
	return err
}
