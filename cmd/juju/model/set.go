// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils/keyvalues"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewSetCommand() cmd.Command {
	return modelcmd.Wrap(&setCommand{})
}

type attributes map[string]interface{}

type setCommand struct {
	modelcmd.ModelCommandBase
	api    SetModelAPI
	values attributes
}

const setModelHelpDoc = `
Model configuration consists of a collection of keys and their respective values.
By default, the model is the current model.
Consult the online documentation for a list of keys and possible values.

Examples:

    juju set-model-config logging-config='<root>=WARNING;unit=INFO'
    juju set-model-config -m mymodel api-port=17071 default-series=xenial

See also: list-models
          get-model-config
          unset-model-config
`

func (c *setCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-model-config",
		Args:    "<model key>=<value> ...",
		Purpose: "Sets configuration keys on a model.",
		Doc:     strings.TrimSpace(setModelHelpDoc),
	}
}

func (c *setCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no key, value pairs specified")
	}

	options, err := keyvalues.Parse(args, true)
	if err != nil {
		return err
	}

	c.values = make(attributes)
	for key, value := range options {
		if key == "agent-version" {
			return fmt.Errorf("agent-version must be set via upgrade-juju")
		}
		c.values[key] = value
	}

	return nil
}

type SetModelAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
	ModelSet(config map[string]interface{}) error
}

func (c *setCommand) getAPI() (SetModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *setCommand) Run(ctx *cmd.Context) error {
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
	for key := range c.values {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current model configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.ModelSet(c.values), block.BlockChange)
}
