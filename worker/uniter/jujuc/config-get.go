package jujuc

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	ctx Context
	Key string // The key to show. If empty, show all.
	out cmd.Output
}

func NewConfigGetCommand(ctx Context) cmd.Command {
	return &ConfigGetCommand{ctx: ctx}
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"config-get", "[<key>]",
		"print service configuration",
		"If a key is given, only the value for that key will be printed.",
	}
}

func (c *ConfigGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if args == nil {
		return nil
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *ConfigGetCommand) Run(ctx *cmd.Context) error {
	cfg, err := c.ctx.Config()
	if err != nil {
		return err
	}
	var value interface{}
	if c.Key == "" {
		value = cfg
	} else {
		value, _ = cfg[c.Key]
	}
	return c.out.Write(ctx, value)
}
