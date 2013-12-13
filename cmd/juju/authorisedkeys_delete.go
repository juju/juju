// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

var deleteKeyDoc = `
Delete an existing authorised ssh key to remove ssh access for the holder of that key.
The key to delete is found by specifying either the "comment" portion of the ssh key,
typically something like "user@host", or the key fingerprint found by using ssh-keygen.
`

// DeleteKeyCommand is used to delete an authorized ssh key for a user.
type DeleteKeyCommand struct {
	cmd.EnvCommandBase
	user  string
	keyId string
}

func (c *DeleteKeyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete",
		Args:    "<ssh key id>",
		Doc:     deleteKeyDoc,
		Purpose: "delete an authorized ssh key for a Juju user",
	}
}

func (c *DeleteKeyCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	case 1:
		c.keyId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *DeleteKeyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.user, "user", "admin", "the user for which to delete the key")
}

func (c *DeleteKeyCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.DeleteKeys(c.user, c.keyId)
	if err != nil {
		return err
	}
	result := results[0]
	return result.Error
}
