// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

var importKeyDoc = `
Import a new authorised ssh key to allow the holder of that key to log on to Juju nodes or machines.
The key is imported using ssh-import-id.
`

// ImportKeyCommand is used to add a new authorized ssh key for a user.
type ImportKeyCommand struct {
	cmd.EnvCommandBase
	user     string
	sshKeyId string
}

func (c *ImportKeyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "import",
		Args:    "<ssh key id>",
		Doc:     importKeyDoc,
		Purpose: "using ssh-import-id, import a new authorized ssh key for a Juju user",
	}
}

func (c *ImportKeyCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key id specified")
	case 1:
		c.sshKeyId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *ImportKeyCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.user, "user", "admin", "the user for which to import the key")
}

func (c *ImportKeyCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.ImportKeys(c.user, c.sshKeyId)
	if err != nil {
		return err
	}
	result := results[0]
	return result.Error
}
