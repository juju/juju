// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs/configstore"
)

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as,
or as an admin, change the password for another user.

Examples:
  # You will be prompted to enter a password.
  juju user change-password

  # Change the password to a random strong password.
  juju user change-password --generate

  # Change the password for bob
  juju user change-password bob --generate

`

// ChangePasswordCommand changes the password for a user.
type ChangePasswordCommand struct {
	UserCommandBase
	Password string
	Generate bool
	OutPath  string
	User     string
}

// Info implements Command.Info.
func (c *ChangePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-password",
		Args:    "",
		Purpose: "changes the password for a user",
		Doc:     userChangePasswordDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ChangePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
	f.StringVar(&c.OutPath, "o", "", "specify the environment file for user")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Init implements Command.Init.
func (c *ChangePasswordCommand) Init(args []string) error {
	var err error
	c.User, err = cmd.ZeroOrOneArgs(args)
	if c.User == "" && c.OutPath != "" {
		return errors.New("output is only a valid option when changing another user's password")
	}
	return err
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

	var credsWriter EnvironInfoCredsWriter
	var creds configstore.APICredentials

	if c.User == "" {
		// We get the creds writer before changing the password just to
		// minimise the things that could go wrong after changing the password
		// in the server.
		credsWriter, err = getEnvironInfoWriter(c)
		if err != nil {
			return errors.Trace(err)
		}

		creds, err = getConnectionCredentials(c)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		creds.User = c.User
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

	if c.User != "" {
		return c.writeEnvironmentFile(ctx)
	}

	credsWriter.SetAPICredentials(creds)
	if err := credsWriter.Write(); err != nil {
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

func (c *ChangePasswordCommand) writeEnvironmentFile(ctx *cmd.Context) error {
	outPath := c.OutPath
	if outPath == "" {
		outPath = c.User + ".jenv"
	}
	outPath = normaliseJenvPath(ctx, outPath)
	if err := generateUserJenv(c.ConnectionName(), c.User, c.Password, outPath); err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "environment file written to %s\n", outPath)
	return nil
}
