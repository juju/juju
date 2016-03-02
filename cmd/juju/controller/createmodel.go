// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewCreateModelCommand returns a command to create an model.
func NewCreateModelCommand() cmd.Command {
	return modelcmd.WrapController(&createModelCommand{
		credentialStore: jujuclient.NewFileCredentialStore(),
	})
}

// createModelCommand calls the API to create a new model.
type createModelCommand struct {
	modelcmd.ControllerCommandBase
	api             CreateModelAPI
	credentialStore jujuclient.CredentialStore

	Name           string
	Owner          string
	CredentialSpec string
	CloudName      string
	CloudType      string
	CredentialName string
	Config         common.ConfigFlag
}

const createModelHelpDoc = `
This command will create another model within the current Juju
Controller. The provider has to match, and the model config must
specify all the required configuration values for the provider.

If configuration values are passed by both extra command line arguments and
the --config option, the command line args take priority.

If creating a model in a controller for which you are not the administrator, the
cloud credentials and authorized ssh keys must be specified. The credentials are
specified using the argument --credential <cloud>:<credential>. The authorized ssh keys
are specified using a --config argument, either authorized=keys=value or via a config yaml file.
 
Any credentials used must be for a cloud with the same provider type as the controller.
Controller administrators do not have to specify credentials or ssh keys; by default, the
credentials and keys used to bootstrap the controller are used if no others are specified.

Examples:

    juju create-model new-model

    juju create-model new-model --config aws-creds.yaml --config image-stream=daily
    
    juju create-model new-model --credential aws:mysekrets --config authorized-keys="ssh-rsa ..."

See Also:
    juju help model share
`

func (c *createModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-model",
		Args:    "<name> [--config key=[value] ...] [--credential <cloud>:<credential>]",
		Purpose: "create an model within the Juju Model Server",
		Doc:     strings.TrimSpace(createModelHelpDoc),
	}
}

func (c *createModelCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Owner, "owner", "", "the owner of the new model if not the current user")
	f.StringVar(&c.CredentialSpec, "credential", "", "the name of the cloud and credentials the new model uses to create cloud resources")
	f.Var(&c.Config, "config", "specify a controller config file, or one or more controller configuration options (--config config.yaml [--config k=v ...])")
}

func (c *createModelCommand) Init(args []string) error {
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
		if cloud, err := cloud.CloudByName(c.CloudName); err != nil {
			return errors.Trace(err)
		} else {
			c.CloudType = cloud.Type
		}
		c.CredentialName = parts[1]
	}
	return nil
}

type CreateModelAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.ModelConfig, error)
	CreateModel(owner string, account, config map[string]interface{}) (params.Model, error)
}

func (c *createModelCommand) getAPI() (CreateModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewModelManagerAPIClient()
}

func (c *createModelCommand) Run(ctx *cmd.Context) error {
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
		cred, _, err := common.GetCredentials(ctx, c.credentialStore, "", c.CredentialName, c.CloudName, c.CloudType)
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
		ctx.Infof("created model %q", c.Name)
	} else {
		ctx.Infof("created model %q for %q", c.Name, c.Owner)
	}

	return nil
}

func (c *createModelCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.ModelConfig) (map[string]interface{}, error) {
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
