// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

const loginDoc = `
After login, a token ("macaroon") will become active. It has an expiration
time of 24 hours. Upon expiration, no further Juju commands can be issued
and the user will be prompted to log in again.

Examples:

    juju login bob

See also: enable-user
          disable-user
          logout

`

// NewLoginCommand returns a new cmd.Command to handle "juju login".
func NewLoginCommand() cmd.Command {
	return modelcmd.WrapController(&loginCommand{
		newLoginAPI: func(args juju.NewAPIConnectionParams) (LoginAPI, ConnectionAPI, error) {
			api, err := juju.NewAPIConnection(args)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			return usermanager.NewClient(api), api, nil
		},
	})
}

// loginCommand changes the password for a user.
type loginCommand struct {
	modelcmd.ControllerCommandBase
	newLoginAPI func(juju.NewAPIConnectionParams) (LoginAPI, ConnectionAPI, error)
	User        string
}

// Info implements Command.Info.
func (c *loginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "login",
		Args:    "[username]",
		Purpose: "Logs a user in to a controller.",
		Doc:     loginDoc,
	}
}

// Init implements Command.Init.
func (c *loginCommand) Init(args []string) error {
	var err error
	c.User, err = cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// LoginAPI provides the API methods that the login command uses.
type LoginAPI interface {
	CreateLocalLoginMacaroon(names.UserTag) (*macaroon.Macaroon, error)
	Close() error
}

// ConnectionAPI provides relevant API methods off the underlying connection.
type ConnectionAPI interface {
	ControllerAccess() string
}

// Run implements Command.Run.
func (c *loginCommand) Run(ctx *cmd.Context) error {
	controllerName := c.ControllerName()
	store := c.ClientStore()

	user := c.User
	if user == "" {
		// TODO(rog) Try macaroon login first before
		// falling back to prompting for username.
		// The username has not been specified, so prompt for it.
		fmt.Fprint(ctx.Stderr, "username: ")
		var err error
		user, err = readLine(ctx.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
		if user == "" {
			return errors.Errorf("you must specify a username")
		}
	}
	if !names.IsValidUserName(user) {
		return errors.NotValidf("user name %q", user)
	}
	userTag := names.NewUserTag(user)

	// Make sure that the client is not already logged in,
	// or if it is, that it is logged in as the specified
	// user.
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if accountDetails != nil && accountDetails.User != userTag.Canonical() {
		return errors.New(`already logged in

Run "juju logout" first before attempting to log in as a different user.
`)
	}
	// Read password from the terminal, and attempt to log in using that.
	fmt.Fprint(ctx.Stderr, "password: ")
	password, err := readPassword(ctx.Stdin)
	fmt.Fprintln(ctx.Stderr)
	if err != nil {
		return errors.Trace(err)
	}
	accountDetails = &jujuclient.AccountDetails{
		User:     userTag.Canonical(),
		Password: password,
	}
	params, err := c.NewAPIConnectionParams(store, controllerName, "", accountDetails)
	if err != nil {
		return errors.Trace(err)
	}
	api, conn, err := c.newLoginAPI(params)
	if err != nil {
		return errors.Annotate(err, "creating API connection")
	}
	defer api.Close()

	// Create a new local login macaroon, and update the account details
	// in the client store, removing the recorded password (if any) and
	// storing the macaroon.
	macaroon, err := api.CreateLocalLoginMacaroon(userTag)
	if err != nil {
		return errors.Annotate(err, "failed to create a temporary credential")
	}
	macaroonJSON, err := macaroon.MarshalJSON()
	if err != nil {
		return errors.Annotate(err, "marshalling temporary credential to JSON")
	}
	accountDetails.Password = ""
	accountDetails.Macaroon = string(macaroonJSON)
	accountDetails.LastKnownAccess = conn.ControllerAccess()
	if err := store.UpdateAccount(controllerName, *accountDetails); err != nil {
		return errors.Annotate(err, "failed to record temporary credential")
	}
	ctx.Infof("You are now logged in to %q as %q.", controllerName, userTag.Canonical())
	return nil
}
