// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show. If empty, show all.
	all bool
	out cmd.Output
}

func NewConfigGetCommand(ctx Context) cmd.Command {
	return &ConfigGetCommand{ctx: ctx}
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "config-get",
		Args:    "[<key>]",
		Purpose: "print service configuration",
		Doc:     "If a key is given, only the value for that key will be printed.",
	}
}

func (c *ConfigGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.all, "a", false, "write also keys without values")
	f.BoolVar(&c.all, "all", false, "")
}

func (c *ConfigGetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = args[0]

	return cmd.CheckEmpty(args[1:])
}

func (c *ConfigGetCommand) Run(ctx *cmd.Context) error {
	settings, err := c.ctx.ConfigSettings()
	if err != nil {
		return err
	}
	var value interface{}
	if c.Key == "" {
		if !c.all {
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
