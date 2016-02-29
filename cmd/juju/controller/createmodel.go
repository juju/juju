// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/yaml.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

// NewCreateModelCommand returns a command to create an model.
func NewCreateModelCommand() cmd.Command {
	return modelcmd.WrapController(&createModelCommand{})
}

// createModelCommand calls the API to create a new model.
type createModelCommand struct {
	modelcmd.ControllerCommandBase
	api CreateEnvironmentAPI

	Name         string
	Owner        string
	ConfigFile   cmd.FileVar
	ConfValues   map[string]string
	configParser func(interface{}) (interface{}, error)
}

const createEnvHelpDoc = `
This command will create another model within the current Juju
Controller. The provider has to match, and the model config must
specify all the required configuration values for the provider. In the cases
of ‘ec2’ and ‘openstack’, the same model variables are checked for the
access and secret keys.

If configuration values are passed by both extra command line arguments and
the --config option, the command line args take priority.

Examples:

    juju create-model new-model

    juju create-model new-model --config=aws-creds.yaml

See Also:
    juju help model share
`

func (c *createModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-model",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create an model within the Juju Model Server",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

func (c *createModelCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Owner, "owner", "", "the owner of the new model if not the current user")
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted file containing model config values")
}

func (c *createModelCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("model name is required")
	}
	c.Name, args = args[0], args[1:]

	values, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}
	c.ConfValues = values

	if c.Owner != "" && !names.IsValidUser(c.Owner) {
		return errors.Errorf("%q is not a valid user", c.Owner)
	}

	if c.configParser == nil {
		c.configParser = common.ConformYAML
	}

	return nil
}

type CreateEnvironmentAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.ModelConfig, error)
	CreateModel(owner string, account, config map[string]interface{}) (params.Model, error)
}

func (c *createModelCommand) getAPI() (CreateEnvironmentAPI, error) {
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

	serverSkeleton, err := client.ConfigSkeleton("", "")
	if err != nil {
		return errors.Trace(err)
	}

	attrs, err := c.getConfigValues(ctx, serverSkeleton)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO we pass nil for the account details until we implement that bit.
	model, err := client.CreateModel(modelOwner, nil, attrs)
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
	// The reading of the config YAML is done in the Run
	// method because the Read method requires the cmd Context
	// for the current directory.
	fileConfig := make(map[string]interface{})
	if c.ConfigFile.Path != "" {
		configYAML, err := c.ConfigFile.Read(ctx)
		if err != nil {
			return nil, errors.Annotate(err, "unable to read config file")
		}

		rawFileConfig := make(map[string]interface{})
		err = yaml.Unmarshal(configYAML, &rawFileConfig)
		if err != nil {
			return nil, errors.Annotate(err, "unable to parse config file")
		}

		conformantConfig, err := c.configParser(rawFileConfig)
		if err != nil {
			return nil, errors.Annotate(err, "unable to parse config file")
		}
		betterConfig, ok := conformantConfig.(map[string]interface{})
		if !ok {
			return nil, errors.New("config must contain a YAML map with string keys")
		}

		fileConfig = betterConfig
	}

	configValues := make(map[string]interface{})
	for key, value := range serverSkeleton {
		configValues[key] = value
	}
	for key, value := range fileConfig {
		configValues[key] = value
	}
	for key, value := range c.ConfValues {
		configValues[key] = value
	}
	configValues["name"] = c.Name

	// TODO: allow version to be specified on the command line and add here.
	cfg, err := config.New(config.UseDefaults, configValues)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cfg.AllAttrs(), nil
}
