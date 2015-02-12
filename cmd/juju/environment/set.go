// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils/keyvalues"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

type attributes map[string]interface{}

type SetCommand struct {
	envcmd.EnvCommandBase
	api    SetEnvironmentAPI
	values attributes
}

const setEnvHelpDoc = `
Updates the environment of a running Juju instance.  Multiple key/value pairs
can be passed on as command line arguments.
`

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set",
		Args:    "key=[value] ...",
		Purpose: "replace environment values",
		Doc:     strings.TrimSpace(setEnvHelpDoc),
	}
}

func (c *SetCommand) Init(args []string) (err error) {
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

type SetEnvironmentAPI interface {
	Close() error
	EnvironmentGet() (map[string]interface{}, error)
	EnvironmentSet(config map[string]interface{}) error
}

func (c *SetCommand) getAPI() (SetEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *SetCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// extra call to the API to retrieve env config
	envAttrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}
	for key := range c.values {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current environment configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.EnvironmentSet(c.values), block.BlockChange)
}
