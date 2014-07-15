// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs/configstore"
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
	UserCommandBase
	User        string
	DisplayName string
	Password    string
	OutPath     string
}

func (c *UserAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<username> [<display name>]",
		Purpose: "adds a user",
		Doc:     userAddCommandDoc,
	}
}

func (c *UserAddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Password, "password", "", "Password for new user")
	f.StringVar(&c.OutPath, "o", "", "Output an environment file for new user")
	f.StringVar(&c.OutPath, "output", "", "")
}

func (c *UserAddCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User, args = args[0], args[1:]
	if len(args) > 0 {
		c.DisplayName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

type addUserAPI interface {
	AddUser(username, displayname, password string) error
	Close() error
}

var getAddUserAPI = func(c *UserAddCommand) (addUserAPI, error) {
	return c.NewUserManagerClient()
}

func (c *UserAddCommand) Run(ctx *cmd.Context) error {
	client, err := getAddUserAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	if c.Password == "" {
		c.Password, err = utils.RandomPassword()
		if err != nil {
			return errors.Annotate(err, "failed to generate password")
		}
	}

	err = client.AddUser(c.User, c.DisplayName, c.Password)
	if err != nil {
		return err
	}
	user := c.User
	if c.DisplayName != "" {
		user = fmt.Sprintf("%s (%s)", c.DisplayName, user)
	}

	fmt.Fprintf(ctx.Stdout, "user %q added with password %q\n", user, c.Password)

	if c.OutPath != "" {
		outPath := NormaliseJenvPath(ctx, c.OutPath)
		err = GenerateUserJenv(c.ConnectionName(), c.User, c.Password, outPath)
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

func GenerateUserJenv(envName, user, password, outPath string) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	storeInfo, err := store.ReadInfo(envName)
	if err != nil {
		return errors.Trace(err)
	}
	outputInfo := configstore.EnvironInfoData{}
	outputInfo.User = user
	outputInfo.Password = password
	outputInfo.StateServers = storeInfo.APIEndpoint().Addresses
	outputInfo.CACert = storeInfo.APIEndpoint().CACert
	yaml, err := cmd.FormatYaml(outputInfo)
	if err != nil {
		return errors.Trace(err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer outFile.Close()
	outFile.Write(yaml)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
