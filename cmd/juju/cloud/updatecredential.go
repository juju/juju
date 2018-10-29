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
Updates a remote credential.`[1:]

var usageUpdateCredentialDetails = `
This command updates a credential cached on a controller. It does this by
uploading the identically named local credential and validating it against the
cloud. Use the ` + "`credentials`" + ` and ` + "`show-credential`" + ` commands to view local and
remote credentials respectively.

This command does not affect what models the updated credential may be related
to. See command ` + "`set-credential`" + ` for that.

This is the only command that can change the contents of an active credential.
It is typically preceded by the ` + "`add-credential --replace`" + `  command.

Examples:

For cloud 'google', update remote credential 'jen':

    juju update-credential google jen

See also: 
    add-credential
    credentials
    set-credential
    show-credential`[1:]

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
