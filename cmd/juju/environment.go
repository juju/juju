package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"strings"
)

// GetEnvironmentCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type GetEnvironmentCommand struct {
	EnvCommandBase
	key string
	out cmd.Output
}

const getEnvHelpDoc = `
If no extra args passed on the command line, all configuration keys and values
for the environment are output using the selected formatter.

A single environment value can be output by adding the environment key name to
the end of the command line.

e.g. $ juju get-environment default-series
     precise
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
	// TODO(thumper) --private to also include private.
}

func (c *GetEnvironmentCommand) Init(args []string) (err error) {
	c.key, err = c.ZeroOrOneArgs(args)
	return
}

func (c *GetEnvironmentCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Get the existing environment config from the state.
	config, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := config.AllAttrs()

	// If no key specified, write out the whole lot.
	if c.key == "" {
		return c.out.Write(ctx, attrs)
	}

	value, found := attrs[c.key]
	if found {
		return c.out.Write(ctx, value)
	}

	return fmt.Errorf("Environment key %q not found in %q environment.", c.key, config.Name())
}

type attributes map[string]interface{}

// SetEnvironment
type SetEnvironmentCommand struct {
	EnvCommandBase
	values attributes
}

const setEnvHelpDoc = `
TODO: write me
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

// SetFlags handled entirely by EnvCommandBase

func (c *SetEnvironmentCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return fmt.Errorf("No key, value pairs specified")
	}
	c.values = make(attributes)
	for i, arg := range args {
		bits := strings.SplitN(arg, "=", 2)
		if len(bits) < 2 {
			return fmt.Errorf(`Missing "=" in arg %d: %q`, i+1, arg)
		}
		key := bits[0]
		if _, exists := c.values[key]; exists {
			return fmt.Errorf(`Key %q specified more than once`, key)
		}
		c.values[key] = bits[1]
	}
	return nil
}

func (c *SetEnvironmentCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Here is the magic around setting the attributes:

	// Get the existing environment config from the state.
	stateConfig, err := conn.State.EnvironConfig()
	if err != nil {
		return err
	}
	// Apply the attributes specified for the command to the state config.
	newConfig, err := stateConfig.Apply(c.values)
	if err != nil {
		return err
	}
	// Now validate this new config against the existing config via the provider.
	provider := conn.Environ.Provider()
	newProviderConfig, err := provider.Validate(newConfig, stateConfig)
	if err != nil {
		return err
	}
	// Now try to apply the new validate config.
	return conn.State.SetEnvironConfig(newProviderConfig)
}
