// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"errors"
	"fmt"

	"github.com/juju/cmd/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageRemoveSSHKeySummary = `
Removes a public SSH key (or keys) from a model.`[1:]

var usageRemoveSSHKeyDetails = `
Juju maintains a per-model cache of public SSH keys which it copies to
each unit. This command will remove a specified key (or space separated
list of keys) from the model cache and all current units deployed in that
model. The keys to be removed may be specified by the key's fingerprint
using a SHA256 sum or by the text label associated with them. Keys may also be
removed by specifying the key verbatim.
`[1:]

const usageRemoveSSHKeyExamples = `
    juju remove-ssh-key ubuntu@ubuntu
    juju remove-ssh-key 45:7f:33:2c:10:4e:6c:14:e3:a1:a4:c8:b2:e1:34:b4
    juju remove-ssh-key bob@ubuntu carol@ubuntu
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
		return block.ProcessBlockedError(ctx, err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(ctx.Stderr, "cannot remove key id %q: %v\n", c.keyIds[i], result.Error)
		}
	}
	return nil
}
