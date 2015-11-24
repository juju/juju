// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"os"
	"os/user"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/yaml.v2"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	localProvider "github.com/juju/juju/provider/local"
)

func newCreateEnvironmentCommand() cmd.Command {
	return envcmd.WrapSystem(&createEnvironmentCommand{})
}

// createEnvironmentCommand calls the API to create a new environment.
type createEnvironmentCommand struct {
	envcmd.SysCommandBase
	api CreateEnvironmentAPI

	Name         string
	Owner        string
	ConfigFile   cmd.FileVar
	ConfValues   map[string]string
	configParser func(interface{}) (interface{}, error)
}

const createEnvHelpDoc = `
This command will create another environment within the current Juju
Environment Server. The provider has to match, and the environment config must
specify all the required configuration values for the provider. In the cases
of ‘ec2’ and ‘openstack’, the same environment variables are checked for the
access and secret keys.

If configuration values are passed by both extra command line arguments and
the --config option, the command line args take priority.

Examples:

    juju system create-environment new-env

    juju system create-environment new-env --config=aws-creds.yaml

See Also:
    juju help environment share
`

func (c *createEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-environment",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create an environment within the Juju Environment Server",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
		Aliases: []string{"create-env"},
	}
}

func (c *createEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Owner, "owner", "", "the owner of the new environment if not the current user")
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted file containing environment config values")
}

func (c *createEnvironmentCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("environment name is required")
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
	ConfigSkeleton(provider, region string) (params.EnvironConfig, error)
	CreateEnvironment(owner string, account, config map[string]interface{}) (params.Environment, error)
}

func (c *createEnvironmentCommand) getAPI() (CreateEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewEnvironmentManagerAPIClient()
}

func (c *createEnvironmentCommand) Run(ctx *cmd.Context) (return_err error) {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	creds, err := c.ConnectionCredentials()
	if err != nil {
		return errors.Trace(err)
	}

	creatingForSelf := true
	envOwner := creds.User
	if c.Owner != "" {
		owner := names.NewUserTag(c.Owner)
		user := names.NewUserTag(creds.User)
		creatingForSelf = owner == user
		envOwner = c.Owner
	}

	var info configstore.EnvironInfo
	var endpoint configstore.APIEndpoint
	if creatingForSelf {
		logger.Debugf("create cache entry for %q", c.Name)
		// Create the configstore entry and write it to disk, as this will error
		// if one with the same name already exists.
		endpoint, err = c.ConnectionEndpoint()
		if err != nil {
			return errors.Trace(err)
		}

		store, err := configstore.Default()
		if err != nil {
			return errors.Trace(err)
		}
		info = store.CreateInfo(c.Name)
		info.SetAPICredentials(creds)
		endpoint.EnvironUUID = ""
		if err := info.Write(); err != nil {
			if errors.Cause(err) == configstore.ErrEnvironInfoAlreadyExists {
				newErr := errors.AlreadyExistsf("environment %q", c.Name)
				return errors.Wrap(err, newErr)
			}
			return errors.Trace(err)
		}
		defer func() {
			if return_err != nil {
				logger.Debugf("error found, remove cache entry")
				e := info.Destroy()
				if e != nil {
					logger.Errorf("could not remove environment file: %v", e)
				}
			}
		}()
	} else {
		logger.Debugf("skipping cache entry for %q as owned %q", c.Name, c.Owner)
	}

	serverSkeleton, err := client.ConfigSkeleton("", "")
	if err != nil {
		return errors.Trace(err)
	}

	attrs, err := c.getConfigValues(ctx, serverSkeleton)
	if err != nil {
		return errors.Trace(err)
	}

	// We pass nil through for the account details until we implement that bit.
	env, err := client.CreateEnvironment(envOwner, nil, attrs)
	if err != nil {
		// cleanup configstore
		return errors.Trace(err)
	}
	if creatingForSelf {
		// update the cached details with the environment uuid
		endpoint.EnvironUUID = env.UUID
		info.SetAPIEndpoint(endpoint)
		if err := info.Write(); err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("created environment %q", c.Name)
		return envcmd.SetCurrentEnvironment(ctx, c.Name)
	} else {
		ctx.Infof("created environment %q for %q", c.Name, c.Owner)
	}

	return nil
}

func (c *createEnvironmentCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.EnvironConfig) (map[string]interface{}, error) {
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

	if err := setConfigSpecialCaseDefaults(c.Name, configValues); err != nil {
		return nil, errors.Trace(err)
	}
	// TODO: allow version to be specified on the command line and add here.
	cfg, err := config.New(config.UseDefaults, configValues)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cfg.AllAttrs(), nil
}

var userCurrent = user.Current

func setConfigSpecialCaseDefaults(envName string, cfg map[string]interface{}) error {
	// As a special case, the local provider's namespace value
	// comes from the user's name and the environment name.
	switch cfg["type"] {
	case "local":
		if _, ok := cfg[localProvider.NamespaceKey]; ok {
			return nil
		}
		username := os.Getenv("USER")
		if username == "" {
			u, err := userCurrent()
			if err != nil {
				return errors.Annotatef(err, "failed to determine username for namespace")
			}
			username = u.Username
		}
		cfg[localProvider.NamespaceKey] = username + "-" + envName
	}
	return nil
}
