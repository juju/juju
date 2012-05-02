package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// ConfigGetCommand implements the `config-get` command. It requires a Context
// whose St and LocalUnitName fields are non-zero.
type ConfigGetCommand struct {
	ctx *Context
	Key string
}

// checkCtx validates that the command's Context is suitable.
func (c *ConfigGetCommand) checkCtx() error {
	if c.ctx.St == nil {
		return fmt.Errorf("context %s cannot access state", c.ctx.Id)
	} else if c.ctx.LocalUnitName == "" {
		return fmt.Errorf("context %s is not attached to a unit", c.ctx.Id)
	}
	return nil
}

var purpose = "write service configuration to stdout"
var doc = `
If <key> is not specified, the full service configuration will be written;
if it is, a single value will be written.
`

// Info returns usage information.
func (c *ConfigGetCommand) Info() *cmd.Info {
	return &cmd.Info{"config-get", "[<key>]", purpose, doc}
}

// Init parses the command line and returns any errors encountered.
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

// Run retrieves the requested information and writes it to stdout.
func (c *ConfigGetCommand) Run(ctx *cmd.Context) error {
	unit, err := c.ctx.St.Unit(c.ctx.LocalUnitName)
	if err != nil {
		return err
	}
	service, err := c.ctx.St.Service(unit.ServiceName())
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
	fmt.Fprintf(ctx.Stdout, "%v\n", value)
	return nil
}
