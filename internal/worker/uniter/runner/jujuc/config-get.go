// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show. If empty, show all.
	All bool
	out cmd.Output
}

func NewConfigGetCommand(ctx Context) (cmd.Command, error) {
	return &ConfigGetCommand{ctx: ctx}, nil
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	doc := `
config-get returns information about the application configuration
(as defined by config.yaml). If called without arguments, it returns
a dictionary containing all config settings that are either explicitly
set, or which have a non-nil default value. If the --all flag is passed,
it returns a dictionary containing all defined config settings including
nil values (for those without defaults). If called with a single argument,
it returns the value of that config key. Missing config keys are reported
as nulls, and do not return an error.

<key> and --all are mutually exclusive.
`
	examples := `
    INTERVAL=$(config-get interval)

    config-get --all
`
	return jujucmd.Info(&cmd.Info{
		Name:     "config-get",
		Args:     "[<key>]",
		Purpose:  "Print application configuration.",
		Doc:      doc,
		Examples: examples,
	})
}

func (c *ConfigGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.BoolVar(&c.All, "a", false, "print all keys")
	f.BoolVar(&c.All, "all", false, "")
}

func (c *ConfigGetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = args[0]
	if c.Key != "" && c.All {
		return fmt.Errorf("cannot use argument --all together with key %q", c.Key)
	}

	return cmd.CheckEmpty(args[1:])
}

func (c *ConfigGetCommand) Run(ctx *cmd.Context) error {
	settings, err := c.ctx.ConfigSettings(ctx)
	if err != nil {
		return err
	}
	var value interface{}
	if c.Key == "" {
		if !c.All {
			for k, v := range settings {
				if v == nil {
					delete(settings, k)
				}
			}
		}
		value = settings
	} else {
		value, _ = settings[c.Key]
	}
	return c.out.Write(ctx, value)
}
