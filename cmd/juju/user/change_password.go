// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/readpass"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/configstore"
)

// randomPasswordNotify is called when a random password is generated.
var randomPasswordNotify = func(string) {}

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as,
or as an admin, change the password for another user.

Examples:
  # You will be prompted to enter a password.
  juju user change-password

  # Change the password to a random strong password.
  juju user change-password --generate

  # Change the password for bob, this always uses a random password
  juju user change-password bob

`

// ChangePasswordCommand changes the password for a user.
type ChangePasswordCommand struct {
	UserCommandBase
	api      ChangePasswordAPI
	writer   EnvironInfoCredsWriter
	Generate bool
	OutPath  string
	User     string
}

// Info implements Command.Info.
func (c *ChangePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-password",
		Args:    "[username]",
		Purpose: "changes the password for a user",
		Doc:     userChangePasswordDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ChangePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
	f.StringVar(&c.OutPath, "o", "", "specifies the path of the generated user environment file")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Init implements Command.Init.
func (c *ChangePasswordCommand) Init(args []string) error {
	var err error
	c.User, err = cmd.ZeroOrOneArgs(args)
	if c.User == "" && c.OutPath != "" {
		return errors.New("output is only a valid option when changing another user's password")
	}
	if c.User != "" {
		c.Generate = true
		if c.OutPath == "" {
			c.OutPath = c.User + ".server"
		}
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
	APICredentials() configstore.APICredentials
	SetAPICredentials(creds configstore.APICredentials)
}

// Run implements Command.Run.
func (c *ChangePasswordCommand) Run(ctx *cmd.Context) error {
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	password, err := c.generateOrReadPassword(ctx, c.Generate)
	if err != nil {
		return errors.Trace(err)
	}

	var writer EnvironInfoCredsWriter

	var creds configstore.APICredentials

	if c.User == "" {
		// We get the creds writer before changing the password just to
		// minimise the things that could go wrong after changing the password
		// in the server.
		if c.writer == nil {
			writer, err = c.ConnectionInfo()
			if err != nil {
				return errors.Trace(err)
			}
		} else {
			writer = c.writer
		}

		creds = writer.APICredentials()
	} else {
		creds.User = c.User
	}

	oldPassword := creds.Password
	creds.Password = password
	if err = c.api.SetPassword(creds.User, password); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	if c.User != "" {
		return writeServerFile(c, ctx, c.User, password, c.OutPath)
	}

	writer.SetAPICredentials(creds)
	if err := writer.Write(); err != nil {
		logger.Errorf("updating the cached credentials failed, reverting to original password")
		setErr := c.api.SetPassword(creds.User, oldPassword)
		if setErr != nil {
			logger.Errorf("failed to set password back, you will need to edit your environments file by hand to specify the password: %q", password)
			return errors.Annotate(setErr, "failed to set password back")
		}
		return errors.Annotate(err, "failed to write new password to environments file")
	}
	ctx.Infof("Your password has been updated.")
	return nil
}

var readPassword = readpass.ReadPassword

func (*ChangePasswordCommand) generateOrReadPassword(ctx *cmd.Context, generate bool) (string, error) {
	if generate {
		password, err := utils.RandomPassword()
		if err != nil {
			return "", errors.Annotate(err, "failed to generate random password")
		}
		randomPasswordNotify(password)
		return password, nil
	}

	// Don't add the carriage returns before readPassword, but add
	// them directly after the readPassword so any errors are output
	// on their own lines.
	fmt.Fprint(ctx.Stdout, "password: ")
	password, err := readPassword()
	fmt.Fprint(ctx.Stdout, "\n")
	if err != nil {
		return "", errors.Trace(err)
	}
	fmt.Fprint(ctx.Stdout, "type password again: ")
	verify, err := readPassword()
	fmt.Fprint(ctx.Stdout, "\n")
	if err != nil {
		return "", errors.Trace(err)
	}
	if password != verify {
		return "", errors.New("Passwords do not match")
	}
	return password, nil
}
