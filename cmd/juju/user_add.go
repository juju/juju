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

const userAddCommandDoc = `
Add users to an existing environment.

The user information is stored within an existing environment, and
will be lost when the environent is destroyed.  A jenv file
identifying the user and the environment will be written to stdout, or
to a path you specify with --output.

Examples:
  juju user add foobar                    (Add user foobar. A strong password will be generated and printed)
  juju user add foobar --password=mypass  (Add user foobar with password "mypass")
  juju user add foobar -o filename        (Add user foobar (with generated password) and save example jenv file to filename)
`

type UserAddCommand struct {
	envcmd.EnvCommandBase
	User     string
	Password string
	out      cmd.Output
}

func (c *UserAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<username> <password>",
		Purpose: "adds a user",
		Doc:     userAddCommandDoc,
	}
}

func (c *UserAddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&(c.Password), "password", "", "Password for new user")
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *UserAddCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return fmt.Errorf("no username supplied")
	case 1:
		c.User = args[0]
	default:
		return cmd.CheckEmpty(args[1:])
	}
	return nil
}

func (c *UserAddCommand) Run(ctx *cmd.Context) error {
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
	if c.Password == "" {
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
