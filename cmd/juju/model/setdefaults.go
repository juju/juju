// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

// NewSetModelDefaultsCommand returns a command used to set default
// model attributes used when creating a new model.
func NewSetModelDefaultsCommand() cmd.Command {
	c := &setDefaultsCommand{}
	c.newAPIFunc = func() (setModelDefaultsAPI, error) {
		api, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Annotate(err, "opening API connection")
		}
		return modelconfig.NewClient(api), nil
	}
	return modelcmd.Wrap(c)
}

type setDefaultsCommand struct {
	modelcmd.ModelCommandBase
	newAPIFunc func() (setModelDefaultsAPI, error)
	values     attributes
}

const setModelDefaultsHelpDoc = `
A shared model configuration attribute is set so that all newly created
models use this value unless overridden.
Consult the online documentation for a list of keys and possible values.

Examples:

    juju set-model-default logging-config='<root>=WARNING;unit=INFO'
    juju set-model-default -m mymodel ftp-proxy=http://proxy default-series=xenial

See also:
    models
    model-defaults
    unset-model-default
`

func (c *setDefaultsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-model-default",
		Args:    "<model key>=<value> ...",
		Purpose: "Sets default configuration keys on a model.",
		Doc:     setModelDefaultsHelpDoc,
	}
}

func (c *setDefaultsCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("no key, value pairs specified")
	}

	options, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}

	c.values = make(attributes)
	for key, value := range options {
		if key == "agent-version" {
			return errors.New("agent-version must be set via upgrade-juju")
		}
		c.values[key] = value
	}
	for key := range c.values {
		// check if the key exists in the known config
		// and warn the user if the key is not defined
		if _, exists := config.ConfigDefaults()[key]; !exists {
			logger.Warningf("key %q is not defined in the known model configuration: possible misspelling", key)
		}
	}

	return nil
}

type setModelDefaultsAPI interface {
	// Close closes the api connection.
	Close() error

	// SetModelDefaults sets the default config values to use
	// when creating new models.
	SetModelDefaults(cloud, region string, config map[string]interface{}) error
}

func (c *setDefaultsCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO(wallyworld) - call with cloud and region when that bit is done
	return block.ProcessBlockedError(client.SetModelDefaults("", "", c.values), block.BlockChange)
}
