// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

// NewAddModelCommand returns a command to add a model.
func NewAddModelCommand() cmd.Command {
	return modelcmd.WrapController(&addModelCommand{
		newAddModelAPI: func(caller base.APICallCloser) AddModelAPI {
			return modelmanager.NewClient(caller)
		},
		newCloudAPI: func(caller base.APICallCloser) CloudAPI {
			return cloudapi.NewClient(caller)
		},
	})
}

// addModelCommand calls the API to add a new model.
type addModelCommand struct {
	modelcmd.ControllerCommandBase
	apiRoot        api.Connection
	newAddModelAPI func(base.APICallCloser) AddModelAPI
	newCloudAPI    func(base.APICallCloser) CloudAPI

	Name           string
	Owner          string
	CredentialName string
	CloudRegion    string
	Config         common.ConfigFlag
}

const addModelHelpDoc = `
Adding a model is typically done in order to run a specific workload. The
model is of the same cloud type as the controller and is managed by that
controller. By default, the controller is the current controller. The
credentials used to add the model are the ones used to create any future
resources within the model (` + "`juju deploy`, `juju add-unit`" + `).

Model names can be duplicated across controllers but must be unique for
any given controller. Model names may only contain lowercase letters,
digits and hyphens, and may not start with a hyphen.

Credential names are specified either in the form "credential-name", or
"credential-owner/credential-name". There is currently no way to acquire
access to another user's credentials, so the only valid value for
credential-owner is your own user name.

Examples:

    juju add-model mymodel
    juju add-model mymodel --config my-config.yaml --config image-stream=daily
    juju add-model mymodel --credential credential_name --config authorized-keys="ssh-rsa ..."
    juju add-model mymodel --region us-east-1
`

func (c *addModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-model",
		Args:    "<model name>",
		Purpose: "Adds a hosted model.",
		Doc:     strings.TrimSpace(addModelHelpDoc),
	}
}

func (c *addModelCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Owner, "owner", "", "The owner of the new model if not the current user")
	f.StringVar(&c.CredentialName, "credential", "", "Credential used to add the model")
	f.StringVar(&c.CloudRegion, "region", "", "Cloud region to add the model to")
	f.Var(&c.Config, "config", "Path to YAML model configuration file or individual options (--config config.yaml [--config key=value ...])")
}

func (c *addModelCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("model name is required")
	}
	c.Name, args = args[0], args[1:]

	if !names.IsValidModelName(c.Name) {
		return errors.Errorf("%q is not a valid name: model names may only contain lowercase letters, digits and hyphens", c.Name)
	}

	if c.Owner != "" && !names.IsValidUser(c.Owner) {
		return errors.Errorf("%q is not a valid user", c.Owner)
	}

	return cmd.CheckEmpty(args)
}

type AddModelAPI interface {
	CreateModel(
		name, owner, cloudRegion string,
		cloudCredential names.CloudCredentialTag,
		config map[string]interface{},
	) (params.ModelInfo, error)
}

type CloudAPI interface {
	DefaultCloud() (names.CloudTag, error)
	Cloud(names.CloudTag) (cloud.Cloud, error)
	Credentials(names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error)
	UpdateCredential(names.CloudCredentialTag, cloud.Credential) error
}

func (c *addModelCommand) newApiRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

func (c *addModelCommand) Run(ctx *cmd.Context) error {
	api, err := c.newApiRoot()
	if err != nil {
		return errors.Annotate(err, "opening API connection")
	}
	defer api.Close()

	store := c.ClientStore()
	controllerName := c.ControllerName()
	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	modelOwner := accountDetails.User
	if c.Owner != "" {
		if !names.IsValidUser(c.Owner) {
			return errors.Errorf("%q is not a valid user name", c.Owner)
		}
		modelOwner = names.NewUserTag(c.Owner).Canonical()
	}
	forUserSuffix := fmt.Sprintf(" for user '%s'", names.NewUserTag(modelOwner).Name())

	attrs, err := c.getConfigValues(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// If the user has specified a credential, then we will upload it if
	// it doesn't already exist in the controller, and it exists locally.
	var credentialTag names.CloudCredentialTag
	if c.CredentialName != "" {
		var err error
		cloudClient := c.newCloudAPI(api)
		credentialTag, err = c.maybeUploadCredential(ctx, cloudClient, modelOwner)
		if err != nil {
			return errors.Trace(err)
		}
	}

	addModelClient := c.newAddModelAPI(api)
	model, err := addModelClient.CreateModel(c.Name, modelOwner, c.CloudRegion, credentialTag, attrs)
	if err != nil {
		return errors.Trace(err)
	}

	messageFormat := "Added '%s' model"
	messageArgs := []interface{}{c.Name}

	if modelOwner == accountDetails.User {
		controllerName := c.ControllerName()
		if err := store.UpdateModel(controllerName, c.Name, jujuclient.ModelDetails{
			model.UUID,
		}); err != nil {
			return errors.Trace(err)
		}
		if err := store.SetCurrentModel(controllerName, c.Name); err != nil {
			return errors.Trace(err)
		}
	}

	if model.CloudRegion != "" {
		messageFormat += " on %s/%s"
		messageArgs = append(messageArgs, controllerDetails.Cloud, model.CloudRegion)
	}
	if model.CloudCredentialTag != "" {
		tag, err := names.ParseCloudCredentialTag(model.CloudCredentialTag)
		if err != nil {
			return errors.Trace(err)
		}
		credentialName := tag.Name()
		if tag.Owner().Canonical() != modelOwner {
			credentialName = fmt.Sprintf("%s/%s", tag.Owner().Canonical(), credentialName)
		}
		messageFormat += " with credential '%s'"
		messageArgs = append(messageArgs, credentialName)
	}

	messageFormat += forUserSuffix

	// lp#1594335
	// "Added '<model>' model [on <cloud>/<region>] [with credential '<credential>'] for user '<user namePart>'"
	ctx.Infof(messageFormat, messageArgs...)

	if _, ok := attrs[config.AuthorizedKeysKey]; !ok {
		// It is not an error to have no authorized-keys when adding a
		// model, though this should never happen since we generate
		// juju-specific SSH keys.
		ctx.Infof(`
No SSH authorized-keys were found. You must use "juju add-ssh-key"
before "juju ssh", "juju scp", or "juju debug-hooks" will work.`)
	}

	return nil
}

func (c *addModelCommand) maybeUploadCredential(
	ctx *cmd.Context,
	cloudClient CloudAPI,
	modelOwner string,
) (names.CloudCredentialTag, error) {

	cloudTag, err := cloudClient.DefaultCloud()
	if err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}
	modelOwnerTag := names.NewUserTag(modelOwner)
	credentialTag, err := common.ResolveCloudCredentialTag(
		modelOwnerTag, cloudTag, c.CredentialName,
	)
	if err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}

	// Check if the credential is already in the controller.
	//
	// TODO(axw) consider implementing a call that can check
	// that the credential exists without fetching all of the
	// names.
	credentialTags, err := cloudClient.Credentials(modelOwnerTag, cloudTag)
	if err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}
	credentialId := credentialTag.Canonical()
	for _, tag := range credentialTags {
		if tag.Canonical() != credentialId {
			continue
		}
		ctx.Infof("using credential '%s' cached in controller", c.CredentialName)
		return credentialTag, nil
	}

	if credentialTag.Owner().Canonical() != modelOwner {
		// Another user's credential was specified, so
		// we cannot automatically upload.
		return names.CloudCredentialTag{}, errors.NotFoundf(
			"credential '%s'", c.CredentialName,
		)
	}

	// Upload the credential from the client, if it exists locally.
	cloudDetails, err := cloudClient.Cloud(cloudTag)
	if err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}
	credential, _, _, err := modelcmd.GetCredentials(
		c.ClientStore(), c.CloudRegion, credentialTag.Name(),
		cloudTag.Id(), cloudDetails.Type,
	)
	if err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}
	ctx.Infof("uploading credential '%s' to controller", credentialTag.Id())
	if err := cloudClient.UpdateCredential(credentialTag, *credential); err != nil {
		return names.CloudCredentialTag{}, errors.Trace(err)
	}
	return credentialTag, nil
}

func (c *addModelCommand) getConfigValues(ctx *cmd.Context) (map[string]interface{}, error) {
	configValues, err := c.Config.ReadAttrs(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "unable to parse config")
	}
	coercedValues, err := common.ConformYAML(configValues)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to parse config")
	}
	attrs, ok := coercedValues.(map[string]interface{})
	if !ok {
		return nil, errors.New("params must contain a YAML map with string keys")
	}
	if err := common.FinalizeAuthorizedKeys(ctx, attrs); err != nil {
		if errors.Cause(err) != common.ErrNoAuthorizedKeys {
			return nil, errors.Trace(err)
		}
	}
	return attrs, nil
}

func canonicalCredentialIds(tags []names.CloudCredentialTag) []string {
	ids := make([]string, len(tags))
	for i, tag := range tags {
		ids[i] = tag.Canonical()
	}
	return ids
}
