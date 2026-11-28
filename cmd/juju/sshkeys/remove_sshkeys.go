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

var usageRemoveSSHKeySummary = `
Removes a public SSH key (or keys) from a model.`[1:]

var usageRemoveSSHKeyDetails = `
Pass each fingerprint, key comment, or full public key as a separate argument.
Quote arguments containing spaces. To select one key, use its fingerprint
from the output of juju ssh-keys.
Check standard error for key errors even if the command exits successfully.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
`[1:]

const usageRemoveSSHKeyExamples = `
Remove the key that matches the contents of a public key file:

    juju remove-ssh-key "$(cat ~/.ssh/id_ed25519.pub)"
`

// NewRemoveKeysCommand is used to delete ssk keys for a user.
func NewRemoveKeysCommand() cmd.Command {
	return modelcmd.Wrap(&removeKeysCommand{})
}

// removeKeysCommand is used to delete authorised ssh keys for a user.
type removeKeysCommand struct {
	SSHKeysBase
	user   string
	keyIds []string
}

// Info implements Command.Info.
func (c *removeKeysCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-ssh-key",
		Args:     "<ssh key id> ...",
		Purpose:  usageRemoveSSHKeySummary,
		Doc:      usageRemoveSSHKeyDetails,
		Examples: usageRemoveSSHKeyExamples,
		SeeAlso: []string{
			"ssh-keys",
			"add-ssh-key",
			"import-ssh-key",
		},
	})
}

// Init implements Command.Init.
func (c *removeKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.keyIds = args
	}
	return nil
}

// Run implements Command.Run.
func (c *removeKeysCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewKeyManagerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.DeleteKeys(ctx, c.user, c.keyIds...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(ctx.Stderr, "cannot remove key id %q: %v\n", c.keyIds[i], result.Error)
		}
	}
	return nil
}
