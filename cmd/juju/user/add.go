// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageSummary = `
Adds a Juju user to a controller.`[1:]

const usageDetails = `The user's details are stored within the controller and
will be removed when the controller is destroyed.

A user unique registration string will be printed. This registration string 
must be used by the newly added user as supplied to 
complete the registration process. 

Some machine providers will require the user to be in possession of certain
credentials in order to create a model.

Examples:
    juju add-user bob
    juju add-user --controller mycontroller bob

See also:
    register
    grant
    users
    show-user
    disable-user
    enable-user
    change-user-password
    remove-user`

// AddUserAPI defines the usermanager API methods that the add command uses.
type AddUserAPI interface {
	AddUser(username, displayName, password string) (names.UserTag, []byte, error)
	Close() error
}

func NewAddCommand() cmd.Command {
	return modelcmd.WrapController(&addCommand{})
}

// addCommand adds new users into a Juju Server.
type addCommand struct {
	modelcmd.ControllerCommandBase
	api         AddUserAPI
	User        string
	DisplayName string
}

// Info implements Command.Info.
func (c *addCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add-user",
		Args:    "<user name> [<display name>]",
		Purpose: usageSummary,
		Doc:     usageDetails,
	})
}

// Init implements Command.Init.
func (c *addCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no username supplied")
	}

	c.User, args = args[0], args[1:]
	if len(args) > 0 {
		c.DisplayName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

// Run implements Command.Run.
func (c *addCommand) Run(ctx *cmd.Context) error {
	api := c.api
	if api == nil {
		var err error
		api, err = c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		defer api.Close()
	}

	// Add a user without a password. This will generate a temporary
	// secret key, which we'll print out for the user to supply to
	// "juju register".
	_, secretKey, err := api.AddUser(c.User, c.DisplayName, "")
	if err != nil {
		if params.IsCodeUnauthorized(err) {
			common.PermissionsMessage(ctx.Stderr, "add a user")
		}
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	displayName := c.User
	if c.DisplayName != "" {
		displayName = fmt.Sprintf("%s (%s)", c.DisplayName, c.User)
	}
	base64RegistrationData, err := generateUserControllerAccessToken(
		c.ControllerCommandBase,
		c.User,
		secretKey,
	)
	if err != nil {
		return errors.Annotate(err, "generating controller user access token")
	}
	fmt.Fprintf(ctx.Stdout, "User %q added\n", displayName)
	fmt.Fprintf(ctx.Stdout, "Please send this command to %v:\n", c.User)
	fmt.Fprintf(ctx.Stdout, "    juju register %s\n",
		base64RegistrationData,
	)
	fmt.Fprintf(ctx.Stdout, `
%q has not been granted access to any models. You can use "juju grant" to grant access.
`, displayName)

	return nil
}
