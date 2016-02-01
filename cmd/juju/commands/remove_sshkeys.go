// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveKeysCommand is used to delete ssk keys for a user.
func NewRemoveKeysCommand() cmd.Command {
	return modelcmd.Wrap(&removeKeysCommand{})
}

var removeKeysDoc = `
Remove existing authorized ssh keys to remove ssh access for the holder of those keys.
The keys to delete are found by specifying either the "comment" portion of the ssh key,
typically something like "user@host", or the key fingerprint found by using ssh-keygen.
`

// removeKeysCommand is used to delete authorised ssh keys for a user.
type removeKeysCommand struct {
	SSHKeysBase
	user   string
	keyIds []string
}

func (c *removeKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-ssh-key",
		Args:    "<ssh key id> [...]",
		Doc:     removeKeysDoc,
		Purpose: "remove authorized ssh keys for a Juju user",
		Aliases: []string {"remove-ssh-keys"},
	}
}

func (c *removeKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.keyIds = args
	}
	return nil
}

func (c *removeKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to remove the keys")
}

func (c *removeKeysCommand) Run(context *cmd.Context) error {
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
			fmt.Fprintf(context.Stderr, "cannot remove key id %q: %v\n", c.keyIds[i], result.Error)
		}
	}
	return nil
}
