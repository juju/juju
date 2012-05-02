package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// ConfigGetCommand implements the config-get command.
type ConfigGetCommand struct {
	ctx *Context
	Key string
}

// checkContext checks that the command has non-zero state and local unit name.
func (c *ConfigGetCommand) checkContext() error {
	if c.ctx.State == nil {
		return fmt.Errorf("context %s cannot access state", c.ctx.Id)
	}
	if c.ctx.LocalUnitName == "" {
		return fmt.Errorf("context %s is not attached to a unit", c.ctx.Id)
	}
	return nil
}

var purpose = "print service configuration"
var doc = "If a key is given, only the value for that key will be printed"

func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{"config-get", "[<key>]", purpose, doc}
}

func (c *ConfigGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
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
	// TODO --format ( = "smart")
	fmt.Fprintln(ctx.Stdout, value)
	return nil
}
