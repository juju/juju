package server

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	ctx *Context
	out resultWriter
	Key string
}

func NewConfigGetCommand(ctx *Context) (cmd.Command, error) {
	if err := ctx.checkUnitState(); err != nil {
		return nil, err
	}
	return &ConfigGetCommand{ctx: ctx}, nil
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"config-get", "[<key>]",
		"print service configuration",
		"If a key is given, only the value for that key will be printed.",
	}
}

func (c *ConfigGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.addFlags(f, "smart", defaultConverters)
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
	unit, err := c.ctx.State.Unit(c.ctx.LocalUnitName)
	if err != nil {
		return err
	}
	service, err := c.ctx.State.Service(unit.ServiceName())
	if err != nil {
		return err
	}
	conf, err := service.Config()
	if err != nil {
		return err
	}
	var value interface{}
	if c.Key == "" {
		value = conf.Map()
	} else {
		value, _ = conf.Get(c.Key)
	}
	return c.out.write(ctx, value)
}
