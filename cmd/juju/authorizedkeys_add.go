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

var addKeysDoc = `
Add new authorized ssh keys to allow the holder of those keys to log on to Juju nodes or machines.
`

// AddKeysCommand is used to add a new authorized ssh key for a user.
type AddKeysCommand struct {
	envcmd.EnvCommandBase
	user    string
	sshKeys []string
}

func (c *AddKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<ssh key> [...]",
		Doc:     addKeysDoc,
		Purpose: "add new authorized ssh keys for a Juju user",
	}
}

func (c *AddKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key specified")
	default:
		c.sshKeys = args
	}
	return nil
}

func (c *AddKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "admin", "the user for which to add the keys")
}

func (c *AddKeysCommand) Run(context *cmd.Context) error {
	client, err := juju.NewKeyManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.AddKeys(c.user, c.sshKeys...)
	if err != nil {
		return err
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot add key %q: %v\n", c.sshKeys[i], result.Error)
		}
	}
	return nil
}
