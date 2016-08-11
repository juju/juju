// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewUnsetCommand() cmd.Command {
	return modelcmd.Wrap(&unsetCommand{})
}

type unsetCommand struct {
	modelcmd.ModelCommandBase
	api  UnsetModelAPI
	keys []string
}

// unsetModelHelpDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const unsetModelHelpDoc = "" +
	"A model key is reset to its default value. If it does not have such a\n" +
	"value defined then it is removed.\n" +
	"Attempting to remove a required key with no default value will result\n" +
	"in an error.\n" +
	"By default, the model is the current model.\n" +
	"Model configuration key values can be viewed with `juju get-model-config`.\n" + unsetModelHelpDocExamples

const unsetModelHelpDocExamples = `
Examples:

    juju unset-model-config ftp-proxy test-mode

See also:
    set-model-config
    get-model-config
`

func (c *unsetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset-model-config",
		Args:    "<model key> ...",
		Purpose: "Unsets model configuration.",
		Doc:     unsetModelHelpDoc,
	}
}

func (c *unsetCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no keys specified")
	}
	c.keys = args
	return nil
}

type UnsetModelAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelUnset(keys ...string) error
}

func (c *unsetCommand) getAPI() (UnsetModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return modelconfig.NewClient(api), nil
}

func (c *unsetCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// extra call to the API to retrieve env config
	envAttrs, err := client.ModelGet()
	if err != nil {
		return err
	}
	for _, key := range c.keys {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current model configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.ModelUnset(c.keys...), block.BlockChange)
}
