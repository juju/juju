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

var importKeysDoc = `
Import new authorised ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
The keys are imported using ssh-import-id.
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
		Args:    "<ssh key id> ...",
		Doc:     importKeysDoc,
		Purpose: "using ssh-import-id, import new authorized ssh keys to a Juju model",
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
