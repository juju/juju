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

func newAddKeysCommand() cmd.Command {
	return envcmd.Wrap(&addKeysCommand{})
}

var addKeysDoc = `
Add new authorised ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
`

// addKeysCommand is used to add a new authorized ssh key for a user.
type addKeysCommand struct {
	AuthorizedKeysBase
	user    string
	sshKeys []string
}

func (c *addKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<ssh key> [...]",
		Doc:     addKeysDoc,
		Purpose: "add new authorized ssh keys for a Juju user",
	}
}

func (c *addKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key specified")
	default:
		c.sshKeys = args
	}
	return nil
}

func (c *addKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to add the keys")
}

func (c *addKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.AddKeys(c.user, c.sshKeys...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot add key %q: %v\n", c.sshKeys[i], result.Error)
		}
	}
	return nil
}
