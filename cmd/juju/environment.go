// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// GetEnvironmentCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetEnvironmentCommand struct {
	envcmd.EnvCommandBase
	key string
	out cmd.Output
}

const getEnvHelpDoc = `
If no extra args passed on the command line, all configuration keys and values
for the environment are output using the selected formatter.

A single environment value can be output by adding the environment key name to
the end of the command line.

Example:
  
  juju get-environment default-series  (returns the default series for the environment)
`

func (c *GetEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-environment",
		Args:    "[<environment key>]",
		Purpose: "view environment values",
		Doc:     strings.TrimSpace(getEnvHelpDoc),
		Aliases: []string{"get-env"},
	}
}

func (c *GetEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *GetEnvironmentCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

func (c *GetEnvironmentCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return fmt.Errorf("key %q not found in %q environment.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

type attributes map[string]interface{}

// SetEnvironment
type SetEnvironmentCommand struct {
	envcmd.EnvCommandBase
	values attributes
}

const setEnvHelpDoc = `
Updates the environment of a running Juju instance.  Multiple key/value pairs
can be passed on as command line arguments.
`

func (c *SetEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-environment",
		Args:    "key=[value] ...",
		Purpose: "replace environment values",
		Doc:     strings.TrimSpace(setEnvHelpDoc),
		Aliases: []string{"set-env"},
	}
}

func (c *SetEnvironmentCommand) Init(args []string) (err error) {
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
		if _, exists := c.values[key]; exists {
			return fmt.Errorf(`key %q specified more than once`, key)
		}

		c.values[key] = value
	}

	return nil
}

func (c *SetEnvironmentCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()

	// extra call to the API to retrieve env config
	envAttrs, err := client.EnvironmentGet()
	if err != nil {
		return err
	}
	for key, _ := range c.values {
		// check if the key exists in the existing env config
		// and warn the user if the key is not defined in
		// the existing config
		if _, exists := envAttrs[key]; !exists {
			logger.Warningf("key %q is not defined in the current environemnt configuration: possible misspelling", key)
		}

	}

	return client.EnvironmentSet(c.values)
}

// UnsetEnvironment
type UnsetEnvironmentCommand struct {
	envcmd.EnvCommandBase
	keys []string
}

const unsetEnvHelpDoc = `
Reset one or more the environment configuration attributes to its default
value in a running Juju instance.  Attributes without defaults are removed,
and attempting to remove a required attribute with no default will result
in an error.

Multiple attributes may be removed at once; keys are space-separated.
`

func (c *UnsetEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset-environment",
		Args:    "<environment key> ...",
		Purpose: "unset environment values",
		Doc:     strings.TrimSpace(unsetEnvHelpDoc),
		Aliases: []string{"unset-env"},
	}
}

func (c *UnsetEnvironmentCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("no keys specified")
	}
	c.keys = args
	return nil
}

func (c *UnsetEnvironmentCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewAPIClient()
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
			logger.Warningf("key %q is not defined in the current environemnt configuration: possible misspelling", key)
		}

	}

	return client.EnvironmentUnset(c.keys...)
}
