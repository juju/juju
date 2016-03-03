// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewAddKeysCommand is used to add a new ssh key to a model.
func NewAddKeysCommand() cmd.Command {
	return modelcmd.Wrap(&addKeysCommand{})
}

var addKeysDoc = `
Add new authorized ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
`

// addKeysCommand is used to add a new authorized ssh key for a user.
type addKeysCommand struct {
	SSHKeysBase
	user    string
	sshKeys []string
}

// Info implements Command.Info.
func (c *addKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-ssh-key",
		Args:    "<ssh key> ...",
		Doc:     addKeysDoc,
		Purpose: "add new authorized ssh key to a Juju model",
		Aliases: []string{"add-ssh-keys"},
	}
}

// Init implements Command.Init.
func (c *addKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key specified")
	default:
		c.sshKeys = args
	}
	return nil
}

// Run implements Command.Run.
func (c *addKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
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
