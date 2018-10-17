// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	apicloud "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageUpdateCredentialSummary = `
Updates a controller credential for a cloud.`[1:]

var usageUpdateCredentialDetails = `
Cloud credentials for controller are used for model operations and manipulations.
Since it is common to have long-running models, it is also common to 
have these cloud credentials become invalid during models' lifetime.
When this happens, a user must update the cloud credential that 
a model was created with to the new and valid details on controller.

This command allows to update an existing, already-stored, named,
cloud-specific controller credential.

NOTE: 
This is the only command that will allow you to manipulate cloud
credential for a controller. 
All other credential related commands, such as 
` + "`add-credential`" + `, ` + "`remove-credential`" + ` and  ` + "`credentials`" + ` 
deal with credentials stored locally on the client not on the controller.

Examples:
    juju update-credential aws mysecrets

See also: 
    add-credential
    credentials`[1:]

type updateCredentialCommand struct {
	modelcmd.ControllerCommandBase

	api credentialAPI

	cloud      string
	credential string
}

// NewUpdateCredentialCommand returns a command to update credential details.
func NewUpdateCredentialCommand() cmd.Command {
	return modelcmd.WrapController(&updateCredentialCommand{})
}

// Init implements Command.Init.
func (c *updateCredentialCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("Usage: juju update-credential <cloud-name> <credential-name>")
	}
	c.cloud = args[0]
	c.credential = args[1]
	return cmd.CheckEmpty(args[2:])
}

// Info implements Command.Info
func (c *updateCredentialCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update-credential",
		Args:    "<cloud-name> <credential-name>",
		Purpose: usageUpdateCredentialSummary,
		Doc:     usageUpdateCredentialDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *updateCredentialCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.StringVar(&c.credential, "credential", "", "Name of credential to update")
}

type credentialAPI interface {
	UpdateCredentialsCheckModels(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error)
	Close() error
}

func (c *updateCredentialCommand) getAPI() (credentialAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return apicloud.NewClient(api), nil
}

// Run implements Command.Run
func (c *updateCredentialCommand) Run(ctx *cmd.Context) error {
	cloud, err := common.CloudByName(c.cloud)
	if errors.IsNotFound(err) {
		ctx.Infof("Cloud %q not found", c.cloud)
		return nil
	} else if err != nil {
		return err
	}
	getCredentialsParams := modelcmd.GetCredentialsParams{
		Cloud:          *cloud,
		CredentialName: c.credential,
	}
	credToUpdate, _, _, err := modelcmd.GetCredentials(ctx, c.ClientStore(), getCredentialsParams)
	if errors.IsNotFound(err) {
		ctx.Infof("No credential called %q exists for cloud %q", c.credential, c.cloud)
		return nil
	} else if err != nil {
		return err
	}
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return err
	}
	credentialTag, err := common.ResolveCloudCredentialTag(
		names.NewUserTag(accountDetails.User), names.NewCloudTag(c.cloud), c.credential,
	)
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := client.UpdateCredentialsCheckModels(credentialTag, *credToUpdate)

	// We always want to display models information if there is any.
	common.OutputUpdateCredentialModelResult(ctx, models, true)
	if err != nil {
		ctx.Infof("Controller credential %q for user %q on cloud %q not updated: %v.", c.credential, accountDetails.User, c.cloud, err)
		// TODO (anastasiamac 2018-09-21) When set-credential is done, also direct user to it.
		// Something along the lines of:
		// "
		// Failed models may require a different credential.
		// Use ‘juju set-credential’ to change credential for these models before repeating this update.
		// "
		//
		// We do not want to return err here as we have already displayed it on the console.
		return cmd.ErrSilent
	}
	ctx.Infof(`
Controller credential %q for user %q on cloud %q updated.
For more information, see ‘juju show-credential %v %v’.`[1:],
		c.credential, accountDetails.User, c.cloud,
		c.cloud, c.credential)
	return nil
}
