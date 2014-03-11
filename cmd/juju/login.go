// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"code.google.com/p/go.crypto/ssh/terminal"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/names"
)

const loginDoc = `
`

type LoginCommand struct {
	envcmd.EnvCommandBase
	User     string
	Password string
}

func (c *LoginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "login",
		Args:    "<username> <password>",
		Purpose: "login as user",
		Doc:     loginDoc,
	}
}

func (c *LoginCommand) Init(args []string) error {
	err := c.EnsureEnvNameSet()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User = args[0]

	if len(args) == 1 {
		fmt.Printf("password: ")
		pass, err := terminal.ReadPassword(0)
		if err != nil {
			return fmt.Errorf("Failed to read password %v", err)
		}
		c.Password = string(pass)
	} else {
		c.Password = args[1]
	}
	return nil
}

func (c *LoginCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return fmt.Errorf("Cannot open client: %v\n", err)
	}
	defer client.Close()
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	c.User = names.UserTag(c.User)
	info, err := store.ReadInfo(c.EnvName)
	if err != nil {
		return err
	}
	creds := configstore.APICredentials{User: c.User, Password: c.Password}

	info.SetAPICredentials(creds)

	conn, err := juju.NewAPIClientFromInfo(c.EnvName, info)
	if err != nil {
		return fmt.Errorf("login failed: %v", err)
	}
	err = conn.Close()
	if err != nil {
		return fmt.Errorf("failed to close connection: %v", err)
	}

	err = info.Write()
	if err != nil {
		return fmt.Errorf("cannot write API credentials: %v", err)
	}
	return nil
}
