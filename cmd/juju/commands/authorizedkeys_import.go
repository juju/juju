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

func newImportKeysCommand() cmd.Command {
	return envcmd.Wrap(&importKeysCommand{})
}

var importKeysDoc = `
Import new authorised ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
The keys are imported using ssh-import-id.
`

// importKeysCommand is used to add new authorized ssh keys for a user.
type importKeysCommand struct {
	AuthorizedKeysBase
	user      string
	sshKeyIds []string
}

func (c *importKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "import",
		Args:    "<ssh key id> [...]",
		Doc:     importKeysDoc,
		Purpose: "using ssh-import-id, import new authorized ssh keys for a Juju user",
	}
}

func (c *importKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.sshKeyIds = args
	}
	return nil
}

func (c *importKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to import the keys")
}

func (c *importKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()

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
