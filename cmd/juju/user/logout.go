// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const logoutDoc = `
If another client has logged in as the same user, they will remain logged
in. This command only affects the local client.

The command will fail if the user has not yet set a password
(` + "`juju change-user-password`" + `). This scenario is only possible after
` + "`juju bootstrap`" + `since ` + "`juju register`" + ` sets a password. The
failing behaviour can be overridden with the ` + "`--force`" + ` option.

If the same user is logged in with another client system, that user session
will not be affected by this command; it only affects the local client.

By default, the controller is the current controller.

`

const logoutExamples = `
    juju logout
`

// NewLogoutCommand returns a new cmd.Command to handle "juju logout".
func NewLogoutCommand() cmd.Command {
	return modelcmd.WrapController(&logoutCommand{})
}

// logoutCommand changes the password for a user.
type logoutCommand struct {
	modelcmd.ControllerCommandBase
	Force bool
}

// Info implements Command.Info.
func (c *logoutCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "logout",
		Purpose:  "Logs a Juju user out of a controller.",
		Doc:      logoutDoc,
		Examples: logoutExamples,
		SeeAlso: []string{
			"change-user-password",
			"login",
		},
	})
}

// Init implements Command.Init.
func (c *logoutCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// SetFlags implements Command.SetFlags.
func (c *logoutCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Force logout when a locally recorded password is detected")
}

// Run implements Command.Run.
func (c *logoutCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()
	if err := c.logout(store, controllerName); err != nil {
		return errors.Trace(err)
	}

	// Count the number of logged-into controllers to inform the user.
	var loggedInCount int
	controllers, err := store.AllControllers()
	if err != nil {
		return errors.Trace(err)
	}
	for name := range controllers {
		if name == controllerName {
			continue
		}
		_, err := store.AccountDetails(name)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		loggedInCount++
	}
	switch loggedInCount {
	case 0:
		ctx.Infof("Logged out. You are no longer logged into any controllers.")
	case 1:
		ctx.Infof("Logged out. You are still logged into 1 controller.")
	default:
		ctx.Infof("Logged out. You are still logged into %d controllers.", loggedInCount)
	}
	return nil
}

func (c *logoutCommand) logout(store jujuclient.ClientStore, controllerName string) error {
	accountDetails, err := store.AccountDetails(controllerName)
	if errors.IsNotFound(err) {
		// Not logged in; nothing else to do.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// We first ensure that the user has a macaroon, which implies
	// they know their password. If they have just bootstrapped,
	// they will have a randomly generated password which they will
	// be unaware of.
	if accountDetails.Password != "" && !c.Force {
		return errors.New(`preventing account loss

It appears that you have not changed the password for
your account. If this is the case, change the password
first before logging out, so that you can log in again
afterwards. To change your password, run the command
"juju change-user-password".

If you are sure you want to log out, and it is safe to
clear the credentials from the client, then you can run
this command again with the "--force" option.
`)
	}

	if err := c.ClearControllerMacaroons(c.ClientStore(), controllerName); err != nil {
		return errors.Trace(err)
	}

	// Remove the account credentials.
	if err := store.RemoveAccount(controllerName); err != nil {
		return errors.Annotate(err, "failed to clear credentials")
	}
	return nil
}
