// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"

	"code.google.com/p/go.crypto/ssh/terminal"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/juju"
)

const addUserDoc = `
Add users to an existing environment
The user information is stored within an existing environment, and will be lost
when the environent is destroyed.

Examples:
  juju add-user foobar mypass       (Add user foobar with password mypass)
  juju add-user foobar              (Add user foobar. A prompt will request the password)
`

type AddUserCommand struct {
	envcmd.EnvCommandBase
	User       string
	Password   string
	OutputFile string
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
	f.StringVar(&c.OutputFile, "file", "", "the file to store the example jenv to")
}
func (c *AddUserCommand) Init(args []string) error {
	err := c.EnsureEnvNameSet()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User = args[0]

	if len(args) == 1 {
		fmt.Print("password: ")
		pass, err := terminal.ReadPassword(0)
		if err != nil {
			return fmt.Errorf("Failed to read password %v", err)
		}
		c.Password = string(pass)
		return nil
	}
	c.Password = args[1]
	return cmd.CheckEmpty(args[2:])
}

func (c *AddUserCommand) Run(_ *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	info, err := store.ReadInfo(c.EnvName)
	if err != nil {
		return err
	}
	client, err := juju.NewUserManagerClient(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	if c.OutputFile != "" {
		outputInfo := configstore.EnvInfo{}
		//info.SetAPICredentials(configstore.APICredentials{c.User, c.Password})
		//info.SetBootstrapConfig(map[string]interface{}{})
		outputInfo.User = c.User
		outputInfo.Password = c.Password
		outputInfo.StateServers = info.APIEndpoint().Addresses
		outputInfo.CACert = info.APIEndpoint().CACert
		data, err := goyaml.Marshal(outputInfo)
		if err != nil {
			return fmt.Errorf("Failed to marshal environ info")
		}
		err = ioutil.WriteFile(c.OutputFile, data, 0777)
		if err != nil {
			return fmt.Errorf("Failed to write user data to file")
		}
	}
	return client.AddUser(c.User, c.Password)
}
