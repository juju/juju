// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"os"
	"os/user"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	localProvider "github.com/juju/juju/provider/local"
)

// CreateCommand calls the API to create a new environment.
type CreateCommand struct {
	envcmd.EnvCommandBase
	api CreateEnvironmentAPI
	// These attributes are exported only for testing purposes.
	Name string
	// TODO: owner string
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

func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create an environment within the Juju Environment Server",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

func (c *CreateCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO: support creating environments for someone else when we work
	// out how to have the other user login and start using the environement.
	// f.StringVar(&c.owner, "owner", "", "the owner of the new environment if not the current user")
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted file containing environment config values")
}

func (c *CreateCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("environment name is required")
	}
	c.Name, args = args[0], args[1:]

	values, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}
	c.ConfValues = values
	return nil
}

type CreateEnvironmentAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.EnvironConfig, error)
	CreateEnvironment(owner string, account, config map[string]interface{}) (params.Environment, error)
}

func (c *CreateCommand) getAPI() (CreateEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return environmentmanager.NewClient(root), nil
}

func (c *CreateCommand) Run(ctx *cmd.Context) (err error) {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// Create the configstore entry and write it to disk, as this will error
	// if one with the same name already exists.
	creds, err := c.ConnectionCredentials()
	if err != nil {
		return errors.Trace(err)
	}
	endpoint, err := c.ConnectionEndpoint(false)
	if err != nil {
		return errors.Trace(err)
	}

	store, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	info := store.CreateInfo(c.Name)
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
		if err != nil {
			e := info.Destroy()
			if e != nil {
				logger.Errorf("could not remove environment file: %v", e)
			}
		}
	}()

	// TODO: support provider and region.
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

	// update the .jenv file with the environment uuid
	endpoint.EnvironUUID = env.UUID
	info.SetAPIEndpoint(endpoint)
	if err := info.Write(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *CreateCommand) getConfigValues(ctx *cmd.Context, serverSkeleton params.EnvironConfig) (map[string]interface{}, error) {
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
