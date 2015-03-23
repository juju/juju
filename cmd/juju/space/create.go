// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/space"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
)

// CreateCommand calls the API to create a new network space.
type CreateCommand struct {
	envcmd.EnvCommandBase
	api CreateSpaceAPI
	// These attributes are exported only for testing purposes.
	Name string
	ConfigFile cmd.FileVar
	ConfValues map[string]string
}

const createEnvHelpDoc = `
This command will create a network space... bla bla bla
`

func (c *CreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> [key=[value] ...]",
		Purpose: "create network space",
		Doc:     strings.TrimSpace(createEnvHelpDoc),
	}
}

func (c *CreateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.ConfigFile, "config", "path to yaml-formatted file containing netowrk space config values")
}

func (c *CreateCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	c.Name, args = args[0], args[1:]

	values, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}
	c.ConfValues = values
	return nil
}

type CreateSpaceAPI interface {
	Close() error
	ConfigSkeleton(provider, region string) (params.EnvironConfig, error)
	CreateSpace(owner string, account, config map[string]interface{}) (params.Space, error)
}

func (c *CreateCommand) getAPI() (CreateSpaceAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return space.NewClient(root), nil
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
			newErr := errors.AlreadyExistsf("space %q", c.Name)
			return errors.Wrap(err, newErr)
		}
		return errors.Trace(err)
	}
	defer func() {
		if err != nil {
			e := info.Destroy()
			if e != nil {
				logger.Errorf("could not remove space file: %v", e)
			}
		}
	}()

	// TODO: support provider and region.
	/*serverSkeleton, err := client.ConfigSkeleton("", "")
	if err != nil {
		return errors.Trace(err)
	}

	attrs, err := c.getConfigValues(ctx, serverSkeleton)
	if err != nil {
		return errors.Trace(err)
	}

	// We pass nil through for the account details until we implement that bit.
	env, err := client.CreateSpace(creds.User, nil, attrs)
	if err != nil {
		// cleanup configstore
		return errors.Trace(err)
	}

	// update the .jenv file with the space uuid
	endpoint.EnvironUUID = env.UUID
	info.SetAPIEndpoint(endpoint)
	if err := info.Write(); err != nil {
		return errors.Trace(err)
	}*/

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

	cfg, err := config.New(config.UseDefaults, configValues)
	if err != nil {
		return nil, errors.Trace(err)
	}

	provider, err := environs.Provider(cfg.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}

	_ = provider

	attrs := cfg.AllAttrs()
	delete(attrs, "agent-version")
	// TODO: allow version to be specified on the command line and add here.

	return attrs, nil
}