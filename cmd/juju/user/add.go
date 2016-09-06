// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"encoding/asn1"
	"encoding/base64"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var usageSummary = `
Adds a Juju user to a controller.`[1:]

var usageDetails = `
A ` + "`juju register`" + ` command will be printed, which must be executed by the
user to complete the registration process. The user's details are stored
within the shared model, and will be removed when the model is destroyed.

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
    remove-user`[1:]

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
	return &cmd.Info{
		Name:    "add-user",
		Args:    "<user name> [<display name>]",
		Purpose: usageSummary,
		Doc:     usageDetails,
	}
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
		return block.ProcessBlockedError(err, block.BlockChange)
	}

	displayName := c.User
	if c.DisplayName != "" {
		displayName = fmt.Sprintf("%s (%s)", c.DisplayName, c.User)
	}

	// Generate the base64-encoded string for the user to pass to
	// "juju register". We marshal the information using ASN.1
	// to keep the size down, since we need to encode binary data.
	controllerDetails, err := c.ClientStore().ControllerByName(c.ControllerName())
	if err != nil {
		return errors.Trace(err)
	}
	registrationInfo := jujuclient.RegistrationInfo{
		User:           c.User,
		Addrs:          controllerDetails.APIEndpoints,
		SecretKey:      secretKey,
		ControllerName: c.ControllerName(),
	}
	registrationData, err := asn1.Marshal(registrationInfo)
	if err != nil {
		return errors.Trace(err)
	}

	// Use URLEncoding so we don't get + or / in the string,
	// and pad with zero bytes so we don't get =; this all
	// makes it easier to copy & paste in a terminal.
	//
	// The embedded ASN.1 data is length-encoded, so the
	// padding will not complicate decoding.
	remainder := len(registrationData) % 3
	for remainder > 0 {
		registrationData = append(registrationData, 0)
		remainder--
	}
	base64RegistrationData := base64.URLEncoding.EncodeToString(
		registrationData,
	)

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
