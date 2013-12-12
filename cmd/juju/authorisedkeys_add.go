// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

var addKeyDoc = `
Add a new authorised ssh key to allow the holder of that key to log on to Juju nodes.
`

// AddKeyCommand is used to add a new authorized ssh key for a user.
type AddKeyCommand struct {
	cmd.EnvCommandBase
	user   string
	sshKey string
}

func (c *AddKeyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<ssh key>",
		Doc:     addKeyDoc,
		Purpose: "add a new authorized ssh key for a Juju user",
	}
}

func (c *AddKeyCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key specified")
	case 1:
		c.sshKey = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *AddKeyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.user, "user", "admin", "the user for which to add the key")
}

func (c *AddKeyCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.AddKeys(c.user, c.sshKey)
	if err != nil {
		return err
	}
	result := results[0]
	return result.Error
}
