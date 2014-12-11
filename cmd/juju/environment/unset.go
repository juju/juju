// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

type UnsetCommand struct {
	envcmd.EnvCommandBase
	api  UnsetEnvironmentAPI
	keys []string
}

const unsetEnvHelpDoc = `
Reset one or more the environment configuration attributes to its default
value in a running Juju instance.  Attributes without defaults are removed,
and attempting to remove a required attribute with no default will result
in an error.

Multiple attributes may be removed at once; keys should be space-separated.
`

func (c *UnsetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset",
		Args:    "<environment key> ...",
		Purpose: "unset environment values",
		Doc:     strings.TrimSpace(unsetEnvHelpDoc),
	}
}

func (c *UnsetCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no keys specified")
	}
	c.keys = args
	return nil
}

type UnsetEnvironmentAPI interface {
	Close() error
	EnvironmentGet() (map[string]interface{}, error)
	EnvironmentUnset(keys ...string) error
}

func (c *UnsetCommand) getAPI() (UnsetEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *UnsetCommand) Run(ctx *cmd.Context) error {
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
	for _, key := range c.keys {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current environment configuration: possible misspelling", key)
		}

	}
	return block.ProcessBlockedError(client.EnvironmentUnset(c.keys...), block.BlockChange)
}
