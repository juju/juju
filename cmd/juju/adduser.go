// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/utils"
)

const addUserDoc = `
Add users to an existing environment
The user information is stored within an existing environment, and will be lost
when the environent is destroyed.
A jenv file identifying the user and the environment will be written to stdout,
or to a path you specify with --output.

Examples:
  juju add-user foobar mypass      (Add user foobar with password mypass)
  juju add-user foobar             (Add user foobar. A strong password will be generated and printed)
  juju add-user foobar -o filename (Add user foobar (with generated password) and save example jenv file to filename)
`

type AddUserCommand struct {
	envcmd.EnvCommandBase
	User             string
	Password         string
	GeneratePassword bool
	out              cmd.Output
}

func (c *AddUserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-user",
		Args:    "<username> <password>",
		Purpose: "adds a user",
		Doc:     addUserDoc,
	}
}

func (c *AddUserCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}
func (c *AddUserCommand) Init(args []string) error {
	err := c.EnvCommandBase.Init()
	if err != nil {
		return err
	}
	switch len(args) {
	case 0:
		return fmt.Errorf("no username supplied")
	case 1:
		c.GeneratePassword = true
	case 2:
		c.Password = args[1]
	default:
		return cmd.CheckEmpty(args[2:])
	}

	c.User = args[0]
	return nil
}

func (c *AddUserCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	storeInfo, err := store.ReadInfo(c.EnvName)
	if err != nil {
		return err
	}
	client, err := juju.NewUserManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	if c.GeneratePassword {
		c.Password, err = utils.RandomPassword()
		if err != nil {
			return fmt.Errorf("Failed to generate password: %v", err)
		}
	}
	outputInfo := configstore.EnvironInfoData{}
	outputInfo.User = c.User
	outputInfo.Password = c.Password
	outputInfo.StateServers = storeInfo.APIEndpoint().Addresses
	outputInfo.CACert = storeInfo.APIEndpoint().CACert
	err = c.out.Write(ctx, outputInfo)
	if err != nil {
		return err
	}
	return client.AddUser(c.User, c.Password)
}
