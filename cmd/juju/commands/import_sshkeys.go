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

// NewImportKeysCommand is used to add new authorized ssh keys to a model.
func NewImportKeysCommand() cmd.Command {
	return modelcmd.Wrap(&importKeysCommand{})
}

// importKeysDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
var importKeysDoc = "" +
	"Juju can add SSH keys to its cache from reliable public sources (currently\n" +
	"Launchpad and Github), allowing those users SSH access to Juju machines.\n" +
	"The user identity supplied should be the username on the respective\n" +
	"service, preferably prefixed by 'lp:' or 'gh:' to avoid confusion/\n" +
	"conflicts.\n" +
	"Note that if the user has multiple keys on the service, all the associated\n" +
	"keys will be added.\n" +
	"Once the keys are added, they can be viewed as normal with the\n" +
	"`juju list-ssh-keys` command, where comments will indicate which ones were\n" +
	"imported this way.\n" + importKeysDocExamples

var importKeysDocExamples = `
Examples:

    juju import-ssh-key lazypower

To specify the service, use a 'gh:' or 'lp:' prefix:

    juju import-ssh-key gh:cherylj

Multiple identities may be specified in a space delimited list:

    juju import-ssh-key rheinlein lp:iasmiov gh:hharrison

See also: add-ssh-key
          list-ssh-keys
`

// importKeysCommand is used to import authorized ssh keys to a model.
type importKeysCommand struct {
	SSHKeysBase
	user      string
	sshKeyIds []string
}

// Info implements Command.Info.
func (c *importKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "import-ssh-key",
		Args:    "<user identity> ...",
		Doc:     importKeysDoc,
		Purpose: "Imports an SSH key from a trusted identity source (Launchpad, Github).",
		Aliases: []string{"import-ssh-keys"},
	}
}

// Init implements Command.Init.
func (c *importKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.sshKeyIds = args
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
