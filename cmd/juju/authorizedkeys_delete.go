// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

var deleteKeysDoc = `
Delete existing authorized ssh keys to remove ssh access for the holder of those keys.
The keys to delete are found by specifying either the "comment" portion of the ssh key,
typically something like "user@host", or the key fingerprint found by using ssh-keygen.
`

// DeleteKeysCommand is used to delete authorized ssh keys for a user.
type DeleteKeysCommand struct {
	envcmd.EnvCommandBase
	user   string
	keyIds []string
}

func (c *DeleteKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete",
		Args:    "<ssh key id> [...]",
		Doc:     deleteKeysDoc,
		Purpose: "delete authorized ssh keys for a Juju user",
	}
}

func (c *DeleteKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.keyIds = args
	}
	return nil
}

func (c *DeleteKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to delete the keys")
}

func (c *DeleteKeysCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.DeleteKeys(c.user, c.keyIds...)
	if err != nil {
		return err
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot delete key id %q: %v\n", c.keyIds[i], result.Error)
		}
	}
	return nil
}
