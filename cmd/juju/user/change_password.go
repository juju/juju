// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs/configstore"
)

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as.

Examples:
  # You will be prompted to enter a password.
  juju user change-password

  # Change the password to a random strong password.
  juju user change-password --generate
`

// ChangePasswordCommand changes the password for the current user.
type ChangePasswordCommand struct {
	UserCommandBase
	Password string
	Generate bool
}

// Info implements Command.Info.
func (c *ChangePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-password",
		Args:    "",
		Purpose: "changes the password of the current user",
		Doc:     userChangePasswordDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ChangePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
}

// ChangePasswordAPI defines the usermanager API methods that the change
// password command uses.
type ChangePasswordAPI interface {
	SetPassword(username, password string) error
	Close() error
}

// EnvironInfoCredsWriter defines methods of the configstore API info that
// are used to change the password.
type EnvironInfoCredsWriter interface {
	Write() error
	SetAPICredentials(creds configstore.APICredentials)
	Location() string
}

func (c *ChangePasswordCommand) getChangePasswordAPI() (ChangePasswordAPI, error) {
	return c.NewUserManagerClient()
}

func (c *ChangePasswordCommand) getEnvironInfoWriter() (EnvironInfoCredsWriter, error) {
	return c.ConnectionWriter()
}

func (c *ChangePasswordCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	return c.ConnectionCredentials()
}

var (
	getChangePasswordAPI     = (*ChangePasswordCommand).getChangePasswordAPI
	getEnvironInfoWriter     = (*ChangePasswordCommand).getEnvironInfoWriter
	getConnectionCredentials = (*ChangePasswordCommand).getConnectionCredentials
)

// Run implements Command.Run.
func (c *ChangePasswordCommand) Run(ctx *cmd.Context) error {
	var err error

	c.Password, err = c.generateOrReadPassword(ctx, c.Generate)
	if err != nil {
		return errors.Trace(err)
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
	err = client.SetPassword(creds.User, c.Password)
	if err != nil {
		return errors.Trace(err)
	}

	info.SetAPICredentials(creds)
	err = info.Write()

	if err != nil {
		logger.Errorf("updating the environments file failed, reverting to original password")
		setErr := client.SetPassword(creds.User, oldPassword)
		if setErr != nil {
			logger.Errorf("failed to set password back, you will need to edit your environments file by hand to specify the password: %q", c.Password)
			return errors.Annotate(setErr, "failed to set password back")
		}
		return errors.Annotate(err, "failed to write new password to environments file")
	}

	ctx.Infof("Your password has been updated.")
	return nil
}
