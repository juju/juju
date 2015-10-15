// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

func newDeleteKeysCommand() cmd.Command {
	return envcmd.Wrap(&deleteKeysCommand{})
}

var deleteKeysDoc = `
Delete existing authorized ssh keys to remove ssh access for the holder of those keys.
The keys to delete are found by specifying either the "comment" portion of the ssh key,
typically something like "user@host", or the key fingerprint found by using ssh-keygen.
`

// deleteKeysCommand is used to delete authorised ssh keys for a user.
type deleteKeysCommand struct {
	AuthorizedKeysBase
	user   string
	keyIds []string
}

func (c *deleteKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete",
		Args:    "<ssh key id> [...]",
		Doc:     deleteKeysDoc,
		Purpose: "delete authorized ssh keys for a Juju user",
	}
}

func (c *deleteKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.keyIds = args
	}
	return nil
}

func (c *deleteKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to delete the keys")
}

func (c *deleteKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.DeleteKeys(c.user, c.keyIds...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot delete key id %q: %v\n", c.keyIds[i], result.Error)
		}
	}
	return nil
}
