// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
)

// GetEnvironmentCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetEnvironmentCommand struct {
	cmd.EnvCommandBase
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
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *GetEnvironmentCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

// environmentGet1dot16 runs matches client.EnvironmentGet using a direct DB
// connection to maintain compatibility with an API server running 1.16 or
// older (when EnvironmentGet was not available). This fallback can be removed
// when we no longer maintain 1.16 compatibility.
func (c *GetEnvironmentCommand) environmentGet1dot16() (map[string]interface{}, error) {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Get the existing environment config from the state.
	config, err := conn.State.EnvironConfig()
	if err != nil {
		return nil, err
	}
	attrs := config.AllAttrs()
	return attrs, nil
}

func (c *GetEnvironmentCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.EnvironmentGet()
	if params.IsCodeNotImplemented(err) {
		logger.Infof("EnvironmentGet not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		attrs, err = c.environmentGet1dot16()
	}
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return fmt.Errorf("Key %q not found in %q environment.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}

type attributes map[string]interface{}

// SetEnvironment
type SetEnvironmentCommand struct {
	cmd.EnvCommandBase
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

// SetFlags handled entirely by cmd.EnvCommandBase

func (c *SetEnvironmentCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("No key, value pairs specified")
	}
	// TODO(thumper) look to have a common library of functions for dealing
	// with key=value pairs.
	c.values = make(attributes)
	for i, arg := range args {
		bits := strings.SplitN(arg, "=", 2)
		if len(bits) < 2 {
			return fmt.Errorf(`Missing "=" in arg %d: %q`, i+1, arg)
		}
		key := bits[0]
		if key == "agent-version" {
			return fmt.Errorf("agent-version must be set via upgrade-juju")
		}
		if _, exists := c.values[key]; exists {
			return fmt.Errorf(`Key %q specified more than once`, key)
		}
		c.values[key] = bits[1]
	}
	return nil
}

// run1dot16 runs matches client.EnvironmentSet using a direct DB
// connection to maintain compatibility with an API server running 1.16 or
// older (when EnvironmentSet was not available). This fallback can be removed
// when we no longer maintain 1.16 compatibility.
// This content was copied from SetEnvironmentCommand.Run in 1.16
func (c *SetEnvironmentCommand) run1dot16() error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Here is the magic around setting the attributes:
	// TODO(thumper): get this magic under test somewhere, and update other call-sites to use it.
	// Get the existing environment config from the state.
	oldConfig, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	// Apply the attributes specified for the command to the state config.
	newConfig, err := oldConfig.Apply(c.values)
	if err != nil {
		return err
	}
	// Now validate this new config against the existing config via the provider.
	provider := conn.Environ.Provider()
	newProviderConfig, err := provider.Validate(newConfig, oldConfig)
	if err != nil {
		return err
	}
	// Now try to apply the new validated config.
	return conn.State.SetEnvironConfig(newProviderConfig, oldConfig)
}

func (c *SetEnvironmentCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.EnvironmentSet(c.values)
	if params.IsCodeNotImplemented(err) {
		logger.Infof("EnvironmentSet not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		err = c.run1dot16()
	}
	return err
}
