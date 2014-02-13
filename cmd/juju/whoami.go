// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

type WhoamiCommand struct {
	cmd.EnvCommandBase
}

func (c *WhoamiCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "whoami",
		Args:    "",
		Purpose: "prints the name of the currently logged in user",
	}
}

func (c *WhoamiCommand) Init(args []string) error {
	return nil
}

func (c *WhoamiCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return nil
}
