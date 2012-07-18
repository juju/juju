package server

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	*ClientContext
	Key string // The key to show. If empty, show all.
	out cmd.Output
}

func NewConfigGetCommand(ctx *ClientContext) (cmd.Command, error) {
	if err := ctx.check(); err != nil {
		return nil, err
	}
	return &ConfigGetCommand{ClientContext: ctx}, nil
}

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"config-get", "[<key>]",
		"print service configuration",
		"If a key is given, only the value for that key will be printed.",
	}
}

func (c *ConfigGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
	f.BoolVar(&c.testMode, "test", false, "returns non-zero exit code if value is false/zero/empty")
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
	unit, err := c.State.Unit(c.LocalUnitName)
	if err != nil {
		return err
	}
	service, err := c.State.Service(unit.ServiceName())
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
	if c.testMode {
		return truthError(value)
	}
	return c.out.Write(ctx, value)
}
