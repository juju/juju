// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/permission"
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

    # Add user with default (read) access to models "qa" and "prod".
    juju add-user foobar --models qa,prod

    # Add user with write access to model "devel".
    juju add-user foobar --models devel --acl write


See Also:
    juju help change-user-password
    juju help register
`

// AddUserAPI defines the usermanager API methods that the add command uses.
type AddUserAPI interface {
	AddUser(username, displayName, password, access string, modelUUIDs ...string) (names.UserTag, []byte, error)
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
	ModelNames  string
	ModelAccess string
}

func (c *addCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ModelNames, "models", "", "models the new user is granted access to")
	f.StringVar(&c.ModelAccess, "acl", "read", "access controls")
}

// Info implements Command.Info.
func (c *addCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-user",
		Args:    "<username> [<display name>] [--models <model1> [<m2> .. <mN>] ] [--acl <read|write>]",
		Purpose: "adds a user to a controller, optionally with access to models",
		Doc:     useraddCommandDoc,
	}
}

// Init implements Command.Init.
func (c *addCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}

	_, err := permission.ParseModelAccess(c.ModelAccess)
	if err != nil {
		return err
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

	var modelNames []string
	for _, modelArg := range strings.Split(c.ModelNames, ",") {
		modelArg = strings.TrimSpace(modelArg)
		if len(modelArg) > 0 {
			modelNames = append(modelNames, modelArg)
		}
	}

	// If we need to share a model, look up the model UUIDs from the supplied names.
	modelUUIDs, err := c.ModelUUIDs(modelNames)
	if err != nil {
		return errors.Trace(err)
	}

	// Add a user without a password. This will generate a temporary
	// secret key, which we'll print out for the user to supply to
	// "juju register".
	_, secretKey, err := api.AddUser(c.User, c.DisplayName, "", c.ModelAccess, modelUUIDs...)
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
		User:      c.User,
		Addrs:     controllerDetails.APIEndpoints,
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
	for _, modelName := range modelNames {
		fmt.Fprintf(ctx.Stdout, "User %q granted %s access to model %q\n", displayName, c.ModelAccess, modelName)
	}
	fmt.Fprintf(ctx.Stdout, "Please send this command to %v:\n", c.User)
	fmt.Fprintf(ctx.Stdout, "    juju register %s\n",
		base64RegistrationData,
	)

	return nil
}
