// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"github.com/juju/names"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/readpass"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs/configstore"
)

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as.

Examples:
juju user change-password               (you will be prompted to enter a password)
juju user change-password --generate    (generate a random strong password)
`

type UserChangePasswordCommand struct {
	UserCommandBase
	Password string
	Generate bool
}

func (c *UserChangePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-password",
		Args:    "",
		Purpose: "changes the password of the current user",
		Doc:     userChangePasswordDoc,
	}
}

func (c *UserChangePasswordCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *UserChangePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
}

type ChangePasswordAPI interface {
	SetPassword(tag names.UserTag, password string) error
	Close() error
}

type EnvironInfoCredsWriter interface {
	Write() error
	SetAPICredentials(creds configstore.APICredentials)
	Location() string
}

var getChangePasswordAPI = func(c *UserChangePasswordCommand) (ChangePasswordAPI, error) {
	return c.NewUserManagerClient()
}

var getEnvironInfoWriter = func(c *UserChangePasswordCommand) (EnvironInfoCredsWriter, error) {
	return c.ConnectionWriter()
}

var getConnectionCredentials = func(c *UserChangePasswordCommand) (configstore.APICredentials, error) {
	return c.ConnectionCredentials()
}

func (c *UserChangePasswordCommand) Run(ctx *cmd.Context) error {
	var err error

	if c.Generate {
		c.Password, err = utils.RandomPassword()
		if err != nil {
			return errors.Annotate(err, "failed to generate random password")
		}
	}

	if c.Password == "" {
		fmt.Println("password:")
		newPass1, err := readpass.ReadPassword()
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Println("type password again:")
		newPass2, err := readpass.ReadPassword()
		if err != nil {
			return errors.Trace(err)
		}
		if newPass1 != newPass2 {
			return errors.New("Passwords do not match")
		}
		c.Password = newPass1
	}

	info, err := getEnvironInfoWriter(c)
	if err != nil {
		return errors.Trace(err)
	}

	creds, err := getConnectionCredentials(c)
	if err != nil {
		return errors.Trace(err)
	}

	client, err := getChangePasswordAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()

	oldPassword := creds.Password
	creds.Password = c.Password

	tag := names.NewUserTag(creds.User)
	err = client.SetPassword(tag, c.Password)
	if err != nil {
		return errors.Trace(err)
	}

	info.SetAPICredentials(creds)

	// TODO (matty) This recovery is not good, will fix in a follow up branch
	err = info.Write()
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "Updating the jenv file failed, reverting to original password\n")
		err = client.SetPassword(tag, oldPassword)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Updating the jenv file failed, reverting failed, you will need to edit your environments file by hand (%s)\n", info.Location())
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stderr, "your password has not changed\n")
	}

	fmt.Fprintf(ctx.Stdout, "your password has been updated\n")

	return nil
}
