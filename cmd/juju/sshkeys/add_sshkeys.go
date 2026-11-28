// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"errors"
	"fmt"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageAddSSHKeySummary = `
Adds a public SSH key to a model.`[1:]

var usageAddSSHKeyDetails = `
Pass the text of each public key as a separate quoted argument, not a filename.
Check standard error: the command can report a key error without a nonzero exit status.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/

`[1:]

const usageAddSSHKeyExamples = `
Add the public key stored in a file:

    juju add-ssh-key "$(cat ~/.ssh/id_ed25519.pub)"

`

// NewAddKeysCommand is used to add a new ssh key to a model.
func NewAddKeysCommand() cmd.Command {
	return modelcmd.Wrap(&addKeysCommand{})
}

// addKeysCommand is used to add a new authorized ssh key for a user.
type addKeysCommand struct {
	SSHKeysBase
	user    string
	sshKeys []string
}

// Info implements Command.Info.
func (c *addKeysCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-ssh-key",
		Args:     "<ssh key> ...",
		Purpose:  usageAddSSHKeySummary,
		Doc:      usageAddSSHKeyDetails,
		Examples: usageAddSSHKeyExamples,
		SeeAlso: []string{
			"ssh-keys",
			"remove-ssh-key",
			"import-ssh-key",
		},
	})
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
func (c *addKeysCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewKeyManagerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.AddKeys(ctx, c.user, c.sshKeys...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(ctx.Stderr, "cannot add key %q: %v\n", c.sshKeys[i], result.Error)
		}
	}
	return nil
}
