// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

var importKeysDoc = `
Import new authorized ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
The keys are imported using ssh-import-id.
`

// ImportKeysCommand is used to add new authorized ssh keys for a user.
type ImportKeysCommand struct {
	envcmd.EnvCommandBase
	user      string
	sshKeyIds []string
}

func (c *ImportKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "import",
		Args:    "<ssh key id> [...]",
		Doc:     importKeysDoc,
		Purpose: "using ssh-import-id, import new authorized ssh keys for a Juju user",
	}
}

func (c *ImportKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	default:
		c.sshKeyIds = args
	}
	return nil
}

func (c *ImportKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to import the keys")
}

func (c *ImportKeysCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.ImportKeys(c.user, c.sshKeyIds...)
	if err != nil {
		return err
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot import key id %q: %v\n", c.sshKeyIds[i], result.Error)
		}
	}
	return nil
}
