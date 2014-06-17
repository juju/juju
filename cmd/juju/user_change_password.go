// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as.

Examples:
juju change-password foobar     (Change password to foobar)
juju change-password            (If no password is specified one is generated)
`

type UserChangePasswordCommand struct {
	envcmd.EnvCommandBase
	Password string
}

func (c *UserChangePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-password",
		Args:    "<password>",
		Purpose: "changes the password of the current user",
		Doc:     userChangePasswordDoc,
	}
}

func (c *UserChangePasswordCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.Password = args[0]
	return cmd.CheckEmpty(args[1:])
}

type ChangePasswordAPI interface {
	SetPassword(username, password string) error
	Close() error
}

var getChangePasswordAPI = func(c *UserChangePasswordCommand) (ChangePasswordAPI, error) {
	return juju.NewUserManagerClient(c.EnvName)
}

var getEnvironInfo = func(c *UserChangePasswordCommand) (configstore.EnvironInfo, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, err
	}
	return store.ReadInfo(c.EnvName)
}

func (c *UserChangePasswordCommand) Run(ctx *cmd.Context) error {
	info, err := getEnvironInfo(c)
	if err != nil {
		return errors.Trace(err)
	}
	user := info.APICredentials().User

	client, err := getChangePasswordAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	if c.Password == "" {
		c.Password, err = utils.RandomPassword()
		if err != nil {
			return errors.Annotate(err, "failed to generate random password")
		}
	}

	err = client.SetPassword(user, c.Password)
	if err != nil {
		return errors.Trace(err)
	}

	info.SetAPICredentials(configstore.APICredentials{
		User:     user,
		Password: c.Password,
	})

	err = info.Write()
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "Updating the jenv file failed, you will need to edit this file by hand with the new password\n")
		return errors.Trace(fmt.Errorf("Failed to write the password back to the .jenv file: %v", err))
	}

	fmt.Fprintf(ctx.Stdout, "your password has been updated\n")

	return nil
}
