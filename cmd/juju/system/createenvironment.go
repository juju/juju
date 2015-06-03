// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/names"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
)

// CreateEnvironmentCommand calls the API to create a new environment.
type CreateEnvironmentCommand struct {
	envcmd.SysCommandBase

	api CreateEnvironmentAPI
	// These attributes are exported only for testing purposes.
	Name  string
	Owner string

	ConfigFile cmd.FileVar
	ConfValues map[string]string
}

const createEnvHelpDoc = `
This command will create another environment within the current Juju
Environment Server. The provider has to match, and the environment config must
specify all the required configuration values for the provider. In the cases
of ‘ec2’ and ‘openstack’, the same environment variables are checked for the
access and secret keys.

If configuration values are passed by both extra command line arguments and
the --config option, the command line args take priority.
`

func (c *CreateEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create-environment",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create an environment within the Juju Environment Server",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

func (c *CreateEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Owner, "owner", "", "the owner of the new environment if not the current user")
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted file containing environment config values")
}

func (c *CreateEnvironmentCommand) Init(args []string) error {
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
	if c.Owner != "" {
		owner := names.NewUserTag(c.Owner)
		user := names.NewUserTag(creds.User)
		creatingForSelf = owner == user
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
	env, err := client.CreateEnvironment(creds.User, nil, attrs)
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
	} else {
		ctx.Infof("created environment %q for %q", c.Name, c.Owner)
	}

	return nil
}

func (c *CreateEnvironmentCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.EnvironConfig) (map[string]interface{}, error) {
	// The reading of the config YAML is done in the Run
	// method because the Read method requires the cmd Context
	// for the current directory.
	fileConfig := make(map[string]interface{})
	if c.ConfigFile.Path != "" {
		configYAML, err := c.ConfigFile.Read(ctx)
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
	for key, value := range c.ConfValues {
		configValues[key] = value
	}
	configValues["name"] = c.Name

	cfg, err := config.New(config.UseDefaults, configValues)
	if err != nil {
		return nil, errors.Trace(err)
	}

	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err = provider.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	attrs := cfg.AllAttrs()
	delete(attrs, "agent-version")
	// TODO: allow version to be specified on the command line and add here.

	return attrs, nil
}
