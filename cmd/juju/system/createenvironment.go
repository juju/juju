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
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	localProvider "github.com/juju/juju/provider/local"
)

// CreateEnvironmentCommand calls the API to create a new environment.
type CreateEnvironmentCommand struct {
	envcmd.SysCommandBase
	api CreateEnvironmentAPI

	name       string
	owner      string
	configFile cmd.FileVar
	confValues map[string]string
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

func (c *CreateEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-environment",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create an environment within the Juju Environment Server",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
		Aliases: []string{"create-env"},
	}
}

func (c *CreateEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.owner, "owner", "", "the owner of the new environment if not the current user")
	f.Var(&c.configFile, "config", "path to yaml-formatted file containing environment config values")
}

func (c *CreateEnvironmentCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("environment name is required")
	}
	c.name, args = args[0], args[1:]

	values, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}
	c.confValues = values

	if c.owner != "" && !names.IsValidUser(c.owner) {
		return errors.Errorf("%q is not a valid user", c.owner)
	}

	return nil
}

type CreateEnvironmentAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.EnvironConfig, error)
	CreateEnvironment(owner string, account, config map[string]interface{}) (params.Environment, error)
}

func (c *CreateEnvironmentCommand) getAPI() (CreateEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewEnvironmentManagerAPIClient()
}

func (c *CreateEnvironmentCommand) Run(ctx *cmd.Context) (return_err error) {
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
	if c.owner != "" {
		owner := names.NewUserTag(c.owner)
		user := names.NewUserTag(creds.User)
		creatingForSelf = owner == user
		envOwner = c.owner
	}

	var info configstore.EnvironInfo
	var endpoint configstore.APIEndpoint
	if creatingForSelf {
		logger.Debugf("create cache entry for %q", c.name)
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
		info = store.CreateInfo(c.name)
		info.SetAPICredentials(creds)
		endpoint.EnvironUUID = ""
		if err := info.Write(); err != nil {
			if errors.Cause(err) == configstore.ErrEnvironInfoAlreadyExists {
				newErr := errors.AlreadyExistsf("environment %q", c.name)
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
		logger.Debugf("skipping cache entry for %q as owned %q", c.name, c.owner)
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
		ctx.Infof("created environment %q", c.name)
		return envcmd.SetCurrentEnvironment(ctx, c.name)
	} else {
		ctx.Infof("created environment %q for %q", c.name, c.owner)
	}

	return nil
}

func (c *CreateEnvironmentCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.EnvironConfig) (map[string]interface{}, error) {
	// The reading of the config YAML is done in the Run
	// method because the Read method requires the cmd Context
	// for the current directory.
	fileConfig := make(map[string]interface{})
	if c.configFile.Path != "" {
		configYAML, err := c.configFile.Read(ctx)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(configYAML, &fileConfig)
		if err != nil {
			return nil, err
		}
	}

	configValues := make(map[string]interface{})
	for key, value := range serverSkeleton {
		configValues[key] = value
	}
	for key, value := range fileConfig {
		configValues[key] = value
	}
	for key, value := range c.confValues {
		configValues[key] = value
	}
	configValues["name"] = c.name

	if err := setConfigSpecialCaseDefaults(c.name, configValues); err != nil {
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
