// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"fmt"

	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageImportSSHKeySummary = `
Imports public SSH keys from GitHub or Launchpad into a model.`[1:]

var usageImportSSHKeyDetails = `
Specify each external account as gh:USERNAME for GitHub or lp:USERNAME for
Launchpad. The command requests the import for the Juju account named admin.
Check standard error for individual source errors even if the command exits successfully.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/
`[1:]

const usageImportSSHKeyExamples = `
With GITHUB_USER set to the GitHub account whose keys you want to authorize:

    juju import-ssh-key "gh:${GITHUB_USER:?set GITHUB_USER first}"
`

// NewImportKeysCommand is used to add new authorized ssh keys to a model.
func NewImportKeysCommand() cmd.Command {
	return modelcmd.Wrap(&importKeysCommand{})
}

// importKeysCommand is used to import authorized ssh keys to a model.
type importKeysCommand struct {
	SSHKeysBase
	user      string
	sshKeyIds []string
}

// Info implements Command.Info.
func (c *importKeysCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "import-ssh-key",
		Args:     "<lp|gh>:<user identity> ...",
		Purpose:  usageImportSSHKeySummary,
		Doc:      usageImportSSHKeyDetails,
		Examples: usageImportSSHKeyExamples,
		SeeAlso: []string{
			"add-ssh-key",
			"ssh-keys",
		},
	})
}

// Init implements Command.Init.
func (c *importKeysCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no ssh key id specified")
	}
	c.sshKeyIds = args
	for _, k := range c.sshKeyIds {
		if len(k) < 3 {
			return errors.NotValidf("%q key ID", k)
		}
		switch k[:3] {
		case "lp:", "gh:":
		default:
			return errors.NewNotSupported(nil,
				fmt.Sprintf("prefix in Key ID %q not supported, only lp: and gh: are allowed", k))
		}
	}
	return nil
}

// Run implemetns Command.Run.
func (c *importKeysCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewKeyManagerClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.ImportKeys(ctx, c.user, c.sshKeyIds...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(ctx.Stderr, "cannot import key id %q: %v\n", c.sshKeyIds[i], result.Error)
		}
	}
	return nil
}
