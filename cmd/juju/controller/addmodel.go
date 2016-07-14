// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"launchpad.net/gnuflag"

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

	return nil
}

type AddModelAPI interface {
	CreateModel(name, owner, cloudRegion, cloudCredential string, config map[string]interface{}) (params.ModelInfo, error)
}

type CloudAPI interface {
	Cloud(names.CloudTag) (cloud.Cloud, error)
	CloudDefaults(names.UserTag) (cloud.Defaults, error)
	Credentials(names.UserTag, names.CloudTag) (map[string]cloud.Credential, error)
	UpdateCredentials(names.UserTag, names.CloudTag, map[string]cloud.Credential) error
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
	if c.CredentialName != "" {
		cloudClient := c.newCloudAPI(api)
		modelOwnerTag := names.NewUserTag(modelOwner)

		defaults, err := cloudClient.CloudDefaults(modelOwnerTag)
		if err != nil {
			return errors.Trace(err)
		}
		cloudTag := names.NewCloudTag(defaults.Cloud)
		credentials, err := cloudClient.Credentials(modelOwnerTag, cloudTag)
		if err != nil {
			return errors.Trace(err)
		}

		if _, ok := credentials[c.CredentialName]; !ok {
			cloudDetails, err := cloudClient.Cloud(cloudTag)
			if err != nil {
				return errors.Trace(err)
			}
			credential, _, _, err := modelcmd.GetCredentials(
				store, c.CloudRegion, c.CredentialName,
				cloudTag.Id(), cloudDetails.Type,
			)
			if err != nil {
				return errors.Trace(err)
			}
			ctx.Infof("uploading credential '%s' to controller%s", c.CredentialName, forUserSuffix)
			credentials = map[string]cloud.Credential{c.CredentialName: *credential}
			if err := cloudClient.UpdateCredentials(modelOwnerTag, cloudTag, credentials); err != nil {
				return errors.Trace(err)
			}
		} else {
			ctx.Infof("using credential '%s' cached in controller", c.CredentialName)
		}
	}

	addModelClient := c.newAddModelAPI(api)
	model, err := addModelClient.CreateModel(c.Name, modelOwner, c.CloudRegion, c.CredentialName, attrs)
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
	if model.CloudCredential != "" {
		messageFormat += " with credential '%s'"
		messageArgs = append(messageArgs, model.CloudCredential)
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
