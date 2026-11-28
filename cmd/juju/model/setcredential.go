// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"context"
	"slices"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// ModelCredentialAPI defines methods used to replace model credential.
type ModelCredentialAPI interface {
	Close() error
	ChangeModelCredential(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error
}

// CloudAPI defines methods used to detemine if cloud credential exists on the controller.
type CloudAPI interface {
	Close() error
	UserCredentials(context.Context, names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error)
	AddCredential(ctx context.Context, tag string, credential cloud.Credential) error
}

// modelCredentialCommand allows to change, replace a cloud credential for a model.
type modelCredentialCommand struct {
	modelcmd.ModelCommandBase

	cloud      string
	credential string

	newAPIRootFunc            func(ctx context.Context) (base.APICallCloser, error)
	newModelCredentialAPIFunc func(base.APICallCloser) ModelCredentialAPI
	newCloudAPIFunc           func(base.APICallCloser) CloudAPI
}

func NewModelCredentialCommand() cmd.Command {
	command := &modelCredentialCommand{
		newModelCredentialAPIFunc: func(root base.APICallCloser) ModelCredentialAPI {
			return modelmanager.NewClient(root)
		},
		newCloudAPIFunc: func(root base.APICallCloser) CloudAPI {
			return cloudapi.NewClient(root)
		},
	}
	command.newAPIRootFunc = func(ctx context.Context) (base.APICallCloser, error) {
		return command.NewControllerAPIRoot(ctx)
	}
	return modelcmd.Wrap(command)
}

// Info implements Command.Info.
func (c *modelCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-credential",
		Args:     "<cloud name> <credential name>",
		Purpose:  "Changes the credential used by one model.",
		Doc:      modelCredentialDoc,
		Examples: modelCredentialExamples,
		SeeAlso: []string{
			"credentials",
			"show-credential",
			"update-credential",
		},
	})
}

// Init implements Command.Init.
func (c *modelCredentialCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.Errorf("Usage: juju set-credential [options] <cloud name> <credential name>")
	}
	if !names.IsValidCloud(args[0]) {
		return errors.NotValidf("cloud name %q", args[0])
	}
	if !names.IsValidCloudCredentialName(args[1]) {
		return errors.NotValidf("cloud credential name %q", args[1])
	}
	c.cloud = args[0]
	c.credential = args[1]
	return nil
}

// Run implements Command.Run.
func (c *modelCredentialCommand) Run(ctx *cmd.Context) error {
	fail := func(e error) error {
		ctx.Infof("Failed to change model credential: %v", e)
		return e
	}

	root, err := c.newAPIRootFunc(ctx)
	if err != nil {
		return fail(errors.Annotate(err, "opening API connection"))
	}
	defer root.Close()

	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return fail(errors.Annotate(err, "getting current account"))
	}
	userTag := names.NewUserTag(accountDetails.User)
	cloudTag := names.NewCloudTag(c.cloud)
	credentialTag, err := common.ResolveCloudCredentialTag(userTag, cloudTag, c.credential)
	if err != nil {
		return fail(errors.Annotate(err, "resolving credential"))
	}

	cloudClient := c.newCloudAPIFunc(root)
	defer cloudClient.Close()

	remote := false
	remoteCredentials, err := cloudClient.UserCredentials(ctx, userTag, cloudTag)
	if err != nil {
		// This is ok - we can proceed with local ones anyway.
		ctx.Infof("Could not determine if there are remote credentials for the user: %v", err)
	} else {
		if slices.Contains(remoteCredentials, credentialTag) {
			remote = true
			ctx.Infof("Found credential remotely, on the controller. Not looking locally...")
		}
	}

	// Credential does not exist remotely. Upload it.
	if !remote {
		ctx.Infof("Did not find credential remotely. Looking locally...")
		credential, err := c.findCredentialLocally(ctx)
		if err != nil {
			return fail((err))
		}
		ctx.Infof("Uploading local credential to the controller.")
		err = cloudClient.AddCredential(ctx, credentialTag.String(), *credential)
		if err != nil {
			return fail(err)
		}
	}

	modelName, modelDetails, err := c.ModelDetails(ctx)
	if err != nil {
		return fail(errors.Trace(err))
	}
	modelTag := names.NewModelTag(modelDetails.ModelUUID)

	modelClient := c.newModelCredentialAPIFunc(root)
	defer modelClient.Close()

	err = modelClient.ChangeModelCredential(ctx, modelTag, credentialTag)
	if err != nil {
		return block.ProcessBlockedError(errors.Annotate(err, "could not set model credential"), block.BlockChange)
	}
	ctx.Infof("Changed cloud credential on model %q to %q.", modelName, c.credential)
	return nil
}

func (c *modelCredentialCommand) findCredentialLocally(ctx *cmd.Context) (*cloud.Credential, error) {
	foundcloud, err := common.CloudByName(c.cloud)
	if err != nil {
		return nil, err
	}
	getCredentialsParams := modelcmd.GetCredentialsParams{
		Cloud:          *foundcloud,
		CredentialName: c.credential,
	}
	credential, _, _, err := modelcmd.GetCredentials(ctx, c.ClientStore(), getCredentialsParams)
	if err != nil {
		return nil, err
	}
	return credential, nil
}

const modelCredentialDoc = `
Choose a credential for the model's cloud under the signed-in account.
The CLI uses a matching controller copy first. If the lookup fails or
finds no match, it tries to upload the local copy before requesting
the switch. A failed lookup does not mean the name is unused: the upload
can replace the material for every model using that cloud, owner and name,
even if the controller refuses the switch. Check both copies before
running this command. To change only one model, use a separately named
credential whose contents you have verified.

The controller requires model admin or controller superuser access and
refuses the switch while model changes are blocked. Use
` + "`juju update-credential`" + ` to replace material shared by several models.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/model/

`

const modelCredentialExamples = `
Switch the model trinity to the AWS credential named bob:

    juju set-credential -m trinity aws bob
`
