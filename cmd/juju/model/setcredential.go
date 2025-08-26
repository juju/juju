// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/client/cloud"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
)

// ModelCredentialAPI defines methods used to replace model credential.
type ModelCredentialAPI interface {
	Close() error
	ChangeModelCredential(model names.ModelTag, credential names.CloudCredentialTag) error
}

// CloudAPI defines methods used to detemine if cloud credential exists on the controller.
type CloudAPI interface {
	Close() error
	UserCredentials(names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error)
	AddCredential(tag string, credential cloud.Credential) error
}

// modelCredentialCommand allows to change, replace a cloud credential for a model.
type modelCredentialCommand struct {
	modelcmd.ModelCommandBase

	cloud      string
	credential string

	newAPIRootFunc            func() (base.APICallCloser, error)
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
	command.newAPIRootFunc = func() (base.APICallCloser, error) {
		return command.NewControllerAPIRoot()
	}
	return modelcmd.Wrap(command)
}

// Info implements Command.Info.
func (c *modelCredentialCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-credential",
		Args:     "<cloud name> <credential name>",
		Purpose:  "Relates a remote credential to a model.",
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

	root, err := c.newAPIRootFunc()
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
	remoteCredentials, err := cloudClient.UserCredentials(userTag, cloudTag)
	if err != nil {
		// This is ok - we can proceed with local ones anyway.
		ctx.Infof("Could not determine if there are remote credentials for the user: %v", err)
	} else {
		for _, credTag := range remoteCredentials {
			if credTag == credentialTag {
				remote = true
				ctx.Infof("Found credential remotely, on the controller. Not looking locally...")
				break
			}
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
		err = cloudClient.AddCredential(credentialTag.String(), *credential)
		if err != nil {
			return fail(err)
		}
	}

	modelName, modelDetails, err := c.ModelDetails()
	if err != nil {
		return fail(errors.Trace(err))
	}
	modelTag := names.NewModelTag(modelDetails.ModelUUID)

	modelClient := c.newModelCredentialAPIFunc(root)
	defer modelClient.Close()

	err = modelClient.ChangeModelCredential(modelTag, credentialTag)
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
This command relates a credential cached on a controller to a specific model.
It does not change/update the contents of an existing active credential. See
command ` + "`update-credential`" + ` for that.

The credential specified may exist locally (on the client), remotely (on the
controller), or both. The command will error out if the credential is stored
neither remotely nor locally.

When remote, the credential will be related to the specified model.

When local and not remote, the credential will first be uploaded to the
controller and then related.

This command does not affect an existing relation between the specified
credential and another model. If the credential is already related to a model
this operation will result in that credential being related to two models.

Use the ` + "`show-credential`" + ` command to see how remote credentials are related
to models.
`

const modelCredentialExamples = `
For cloud ` + "`aws`" + `, relate remote credential ` + "`bob`" + ` to model ` + "`trinity`" + `:

    juju set-credential -m trinity aws bob
`
