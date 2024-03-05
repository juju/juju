// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageImportSSHKeySummary = `
Adds a public SSH key from a trusted identity source to a model.`[1:]

var usageImportSSHKeyDetails = `
Juju can add SSH keys to its cache from reliable public sources (currently
Launchpad and GitHub), allowing those users SSH access to Juju machines.

The user identity supplied is the username on the respective service given by
'lp:' or 'gh:'.

If the user has multiple keys on the service, all the keys will be added.

Once the keys are imported, they can be viewed with the `[1:] + "`juju ssh-keys`" + `
command, where comments will indicate which ones were imported in
this way.

An alternative to this command is the more manual ` + "`juju add-ssh-key`" + `.

`

const usageImportSSHKeyExamples = `
Import all public keys associated with user account 'phamilton' on the
GitHub service:

    juju import-ssh-key gh:phamilton

Multiple identities may be specified in a space delimited list:

    juju import-ssh-key gh:rheinlein lp:iasmiov gh:hharrison
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
func (c *importKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.ImportKeys(c.user, c.sshKeyIds...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot import key id %q: %v\n", c.sshKeyIds[i], result.Error)
		}
	}
	return nil
}
