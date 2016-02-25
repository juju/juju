// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"encoding/asn1"
	"encoding/base64"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

const useraddCommandDoc = `
Add users to a controller to allow them to login to the controller.
Optionally, share a model hosted by the controller with the user.

The user's details are stored with the model being shared, and will be
removed when the model is destroyed.  A "juju register" command will be
printed, which must be executed to complete the user registration process,
setting the user's initial password.

Examples:
    # Add user "foobar"
    juju add-user foobar
    
    # Add user and share model.
    juju add-user foobar --share somemodel


See Also:
    juju help change-user-password
    juju help register
`

// AddUserAPI defines the usermanager API methods that the add command uses.
type AddUserAPI interface {
	AddUser(username, displayName, password string, modelUUIDs ...string) (names.UserTag, []byte, error)
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
	ModelName   string
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ModelName, "share", "", "share a model with the new user")
}

// Info implements Command.Info.
func (c *addCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-user",
		Args:    "<username> [<display name>] [--share <model name>]",
		Purpose: "adds a user to a controller and optionally shares a model",
		Doc:     useraddCommandDoc,
	}
}

// Init implements Command.Init.
func (c *addCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
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

	// If we need to share a model, look up the model UUID from the supplied name.
	var modelUUIDs []string
	if c.ModelName != "" {
		store := c.ClientStore()
		controllerName := c.ControllerName()
		accountName := c.AccountName()
		model, err := store.ModelByName(controllerName, accountName, c.ModelName)
		if errors.IsNotFound(err) {
			// The model isn't known locally, so query the models available in the controller.
			ctx.Verbosef("model %q not cached locally, refreshing models from controller", c.ModelName)
			if err := c.RefreshModels(store, controllerName, accountName); err != nil {
				return errors.Annotate(err, "refreshing models")
			}
			model, err = store.ModelByName(controllerName, accountName, c.ModelName)
		}
		if err != nil {
			return err
		}
		modelUUIDs = []string{model.ModelUUID}
	}

	// Add a user without a password. This will generate a temporary
	// secret key, which we'll print out for the user to supply to
	// "juju register".
	_, secretKey, err := api.AddUser(c.User, c.DisplayName, "", modelUUIDs...)
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
	info, err := c.ConnectionInfo()
	if err != nil {
		return errors.Trace(err)
	}
	registrationInfo := jujuclient.RegistrationInfo{
		User:      c.User,
		Addrs:     info.APIEndpoint().Addresses,
		SecretKey: secretKey,
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
	if c.ModelName != "" {
		fmt.Fprintf(ctx.Stdout, "Model  %q is now shared\n", c.ModelName)
	}
	fmt.Fprintf(ctx.Stdout, "Please send this command to %v:\n", c.User)
	fmt.Fprintf(ctx.Stdout, "    juju register %s\n",
		base64RegistrationData,
	)

	return nil
}
