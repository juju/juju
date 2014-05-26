// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

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
will be lost when the environent is destroyed.  An environment file
(.jenv) identifying the new user and the environment can be generated
using --output.

Examples:
  juju user add foobar                    (Add user "foobar". A strong password will be generated and printed)
  juju user add foobar --password=mypass  (Add user "foobar" with password "mypass")
  juju user add foobar --output filename  (Add user "foobar" and save environment file to "filename")
`

type UserAddCommand struct {
	envcmd.EnvCommandBase
	User     string
	Password string
	outPath  string
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
	f.StringVar(&(c.outPath), "o", "", "Output an environment file for new user")
	f.StringVar(&(c.outPath), "output", "", "")
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

	err = client.AddUser(c.User, c.Password)
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "user \"%s\" added with password \"%s\"\n", c.User, c.Password)

	if c.outPath != "" {
		outPath := NormaliseJenvPath(ctx, c.outPath)
		err := GenerateUserJenv(c.EnvName, c.User, c.Password, outPath)
		if err == nil {
			fmt.Fprintf(ctx.Stdout, "environment file written to %s\n", outPath)
		}
	}
	return err
}

func NormaliseJenvPath(ctx *cmd.Context, outPath string) string {
	if !strings.HasSuffix(outPath, ".jenv") {
		outPath = outPath + ".jenv"
	}
	return ctx.AbsPath(outPath)
}

func GenerateUserJenv(envName, user, password, outPath string) (err error) {
	store, err := configstore.Default()
	if err != nil {
		return
	}
	storeInfo, err := store.ReadInfo(envName)
	if err != nil {
		return
	}
	outputInfo := configstore.EnvironInfoData{}
	outputInfo.User = user
	outputInfo.Password = password
	outputInfo.StateServers = storeInfo.APIEndpoint().Addresses
	outputInfo.CACert = storeInfo.APIEndpoint().CACert
	yaml, err := cmd.FormatYaml(outputInfo)
	if err != nil {
		return
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return
	}
	defer outFile.Close()
	_, err = outFile.Write(yaml)
	if err != nil {
		return
	}
	return
}
