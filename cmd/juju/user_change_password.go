// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/readpass"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as.

Examples:
juju change-password                    (If no password is specified you will be prompted for one)
juju change-password --password foobar  (Change password to foobar)
juju change-password --generate         (Generate a random strong password)
`

type UserChangePasswordCommand struct {
	envcmd.EnvCommandBase
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
	f.StringVar(&c.Password, "password", "", "New password")
	f.BoolVar(&c.Generate, "generate", false, "Generate a new strong password")
}

type ChangePasswordAPI interface {
	SetPassword(username, password string) error
	Close() error
}

type EnvironInfoCredsWriter interface {
	Write() error
	SetAPICredentials(creds configstore.APICredentials)
	APICredentials() configstore.APICredentials
	Location() string
}

var getChangePasswordAPI = func(c *UserChangePasswordCommand) (ChangePasswordAPI, error) {
	return juju.NewUserManagerClient(c.EnvName)
}

var getEnvironInfo = func(c *UserChangePasswordCommand) (EnvironInfoCredsWriter, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, err
	}
	return store.ReadInfo(c.EnvName)
}

func (c *UserChangePasswordCommand) Run(ctx *cmd.Context) error {
	var err error
	if c.Password != "" && c.Generate {
		return fmt.Errorf("You need to choose a password or generate one")
	}

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
			return fmt.Errorf("Passwords do not match")
		}
		c.Password = newPass1
	}

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

	oldPassword := info.APICredentials().Password

	err = client.SetPassword(user, c.Password)
	if err != nil {
		return errors.Trace(err)
	}

	info.SetAPICredentials(configstore.APICredentials{
		User:     user,
		Password: c.Password,
	})

	// TODO (matty) This recovery is not good, will fix in a follow up branch
	err = info.Write()
	if err != nil {
		fmt.Fprintf(ctx.Stderr, "Updating the jenv file failed, reverting to original password\n")
		err = client.SetPassword(user, oldPassword)
		if err != nil {
			fmt.Fprintf(ctx.Stderr, "Updating the jenv file failed, reverting failed, you will need to edit your environments file by hand (%s)\n", info.Location())
			if c.Generate {
				fmt.Fprintf(ctx.Stderr, "Your generated password: %s\n", c.Password)
			}
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stderr, "your password has not changed\n")
	}

	fmt.Fprintf(ctx.Stdout, "your password has been updated\n")

	return nil
}
