// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewAddModelCommand returns a command to add a model.
func NewAddModelCommand() cmd.Command {
	return modelcmd.WrapController(&addModelCommand{
		credentialStore: jujuclient.NewFileCredentialStore(),
	})
}

// addModelCommand calls the API to add a new model.
type addModelCommand struct {
	modelcmd.ControllerCommandBase
	api             AddModelAPI
	credentialStore jujuclient.CredentialStore

	Name           string
	Owner          string
	CredentialSpec string
	CloudName      string
	CloudType      string
	CredentialName string
	Config         common.ConfigFlag
}

const addModelHelpDoc = `
Adding a model is typically done in order to run a specific workload. The
model is of the same cloud type as the controller and resides within that
controller. By default, the controller is the current controller.
The credentials used to add the model are the ones used to create any
future resources within the model (` + "`juju deploy`, `juju add-unit`" + `).
Model names can be duplicated across controllers but must be unique for
any given controller.
The necessary configuration must be available, either via the controller
configuration (known to Juju upon its creation), command line arguments,
or configuration file (--config). For 'ec2' and 'openstack' cloud types,
the access and secret keys need to be provided.
If the same configuration values are passed by both command line arguments
and the --config option, the former take priority.

Examples:

    juju add-model mymodel
    juju add-model mymodel --config aws-creds.yaml --config image-stream=daily
    juju add-model mymodel --credential aws:credential_name --config authorized-keys="ssh-rsa ..."
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
	f.StringVar(&c.CredentialSpec, "credential", "", "Credential used to add the model: <cloud>:<credential name>")
	f.Var(&c.Config, "config", "Path to YAML model configuration file or individual options (--config config.yaml [--config key=value ...])")
}

func (c *addModelCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("model name is required")
	}
	c.Name, args = args[0], args[1:]

	if c.Owner != "" && !names.IsValidUser(c.Owner) {
		return errors.Errorf("%q is not a valid user", c.Owner)
	}

	if c.CredentialSpec != "" {
		parts := strings.Split(c.CredentialSpec, ":")
		if len(parts) < 2 {
			return errors.Errorf("invalid cloud credential %s, expected <cloud>:<credential-name>", c.CredentialSpec)
		}
		c.CloudName = parts[0]
		if cloud, err := common.CloudOrProvider(c.CloudName, cloud.CloudByName); err != nil {
			return errors.Trace(err)
		} else {
			c.CloudType = cloud.Type
		}
		c.CredentialName = parts[1]
	}
	return nil
}

type AddModelAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.ModelConfig, error)
	CreateModel(owner string, account, config map[string]interface{}) (params.Model, error)
}

func (c *addModelCommand) getAPI() (AddModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *addModelCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	store := c.ClientStore()
	controllerName := c.ControllerName()
	accountName, err := store.CurrentAccount(controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	currentAccount, err := store.AccountByName(controllerName, accountName)
	if err != nil {
		return errors.Trace(err)
	}

	modelOwner := currentAccount.User
	if c.Owner != "" {
		if !names.IsValidUser(c.Owner) {
			return errors.Errorf("%q is not a valid user name", c.Owner)
		}
		modelOwner = names.NewUserTag(c.Owner).Canonical()
	}

	serverSkeleton, err := client.ConfigSkeleton(c.CloudType, "")
	if err != nil {
		return errors.Trace(err)
	}

	attrs, err := c.getConfigValues(ctx, serverSkeleton)
	if err != nil {
		return errors.Trace(err)
	}

	accountDetails := map[string]interface{}{}
	if c.CredentialName != "" {
		cred, _, _, err := modelcmd.GetCredentials(
			c.credentialStore, "", c.CredentialName, c.CloudName, c.CloudType,
		)
		if err != nil {
			return errors.Trace(err)
		}
		for k, v := range cred.Attributes() {
			accountDetails[k] = v
		}
	}
	model, err := client.CreateModel(modelOwner, accountDetails, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	if modelOwner == currentAccount.User {
		controllerName := c.ControllerName()
		accountName := c.AccountName()
		if err := store.UpdateModel(controllerName, accountName, c.Name, jujuclient.ModelDetails{
			model.UUID,
		}); err != nil {
			return errors.Trace(err)
		}
		if err := store.SetCurrentModel(controllerName, accountName, c.Name); err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("added model %q", c.Name)
	} else {
		ctx.Infof("added model %q for %q", c.Name, c.Owner)
	}

	return nil
}

func (c *addModelCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.ModelConfig) (map[string]interface{}, error) {
	configValues := make(map[string]interface{})
	for key, value := range serverSkeleton {
		configValues[key] = value
	}
	configAttr, err := c.Config.ReadAttrs(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "unable to parse config")
	}
	for key, value := range configAttr {
		configValues[key] = value
	}
	configValues["name"] = c.Name
	coercedValues, err := common.ConformYAML(configValues)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to parse config")
	}
	stringParams, ok := coercedValues.(map[string]interface{})
	if !ok {
		return nil, errors.New("params must contain a YAML map with string keys")
	}

	return stringParams, nil
}
