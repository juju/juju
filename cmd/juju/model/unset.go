// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewUnsetCommand() cmd.Command {
	return modelcmd.Wrap(&unsetCommand{})
}

type unsetCommand struct {
	modelcmd.ModelCommandBase
	api  UnsetEnvironmentAPI
	keys []string
}

// unsetEnvHelpDoc is multi-line since we need to use ` to denote
// commands for ease in markdown.
const unsetEnvHelpDoc = "" +
	"Unsets specified model configuration to default values and removes\n" +
	"specified keys that have no default values.  Attempting to remove a\n" +
	"required key with no default value will result in an error.\n" +
	"By default, the model is the current model.\n" +
	"Model configuration key values can be viewed with `juju get-model-config`.\n" + unsetEnvHelpDocExamples

const unsetEnvHelpDocExamples = `
Examples:

    juju unset-model-config api-port test-mode

See also: set-model-config
          get-model-config
`

func (c *unsetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset-model-config",
		Args:    "<model key> ...",
		Purpose: "Unsets specified model configuration.",
		Doc:     strings.TrimSpace(unsetEnvHelpDoc),
	}
}

func (c *unsetCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no keys specified")
	}
	c.keys = args
	return nil
}

type UnsetEnvironmentAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelUnset(keys ...string) error
}

func (c *unsetCommand) getAPI() (UnsetEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
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
