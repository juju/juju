// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show. If empty, show all.
	All bool
	out cmd.Output
}

func NewConfigGetCommand(ctx Context) cmd.Command {
	return &ConfigGetCommand{ctx: ctx}
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	doc := `
If a key is given, only the value for that key will be printed. Otherwise
all keys with a value are printed. With the argument --all also empty keys
are printed.
`
	return &cmd.Info{
		Name:    "config-get",
		Args:    "[<key>]",
		Purpose: "print service configuration",
		Doc:     doc,
	}
}

func (c *ConfigGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
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
	settings, err := c.ctx.ConfigSettings()
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
