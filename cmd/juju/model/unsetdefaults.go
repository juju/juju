// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

// NewUnsetModelDefaultsCommand returns a command used to reset default
// model attributes used when creating a new model.
func NewUnsetModelDefaultsCommand() cmd.Command {
	c := &unsetDefaultsCommand{}
	c.newAPIFunc = func() (unsetModelDefaultsAPI, error) {
		api, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Annotate(err, "opening API connection")
		}
		return modelconfig.NewClient(api), nil
	}
	return modelcmd.Wrap(c)

}

type unsetDefaultsCommand struct {
	modelcmd.ModelCommandBase
	newAPIFunc func() (unsetModelDefaultsAPI, error)
	keys       []string
}

// unsetModelDefaultsHelpDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const unsetModelDefaultsHelpDoc = `
A shared model configuration attribute is unset so that all newly created
models will use any Juju defined default.
Consult the online documentation for a list of keys and possible values.

Examples:

    juju unset-model-default ftp-proxy test-mode

See also:
    set-model-config
    get-model-config
`

func (c *unsetDefaultsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset-model-default",
		Args:    "<model key> ...",
		Purpose: "Unsets default model configuration.",
		Doc:     unsetModelDefaultsHelpDoc,
	}
}

func (c *unsetDefaultsCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no keys specified")
	}
	c.keys = args

	for _, key := range c.keys {
		// check if the key exists in the known config
		// and warn the user if the key is not defined
		if _, exists := config.ConfigDefaults()[key]; !exists {
			logger.Warningf("key %q is not defined in the known model configuration: possible misspelling", key)
		}
	}

	return nil
}

type unsetModelDefaultsAPI interface {
	// Close closes the api connection.
	Close() error

	// UnsetModelDefaults clears the default model
	// configuration values.
	UnsetModelDefaults(cloud, region string, keys ...string) error
}

func (c *unsetDefaultsCommand) Run(ctx *cmd.Context) error {
	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO(wallyworld) - call with cloud and region when that bit is done
	return block.ProcessBlockedError(client.UnsetModelDefaults("", "", c.keys...), block.BlockChange)
}
