package server

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

// UnitGetCommand implements the unit-get command.
type UnitGetCommand struct {
	ctx *Context
	out resultWriter
	Arg string
}

func NewUnitGetCommand(ctx *Context) (cmd.Command, error) {
	if err := ctx.checkUnitState(); err != nil {
		return nil, err
	}
	return &UnitGetCommand{ctx: ctx}, nil
}

func (c *UnitGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"unit-get", "<setting>", "print public-address or private-address", "",
	}
}

func (c *UnitGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.addFlags(f, "smart", defaultConverters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if args == nil {
		return errors.New("no setting specified")
	}
	if args[0] != "private-address" && args[0] != "public-address" {
		return fmt.Errorf("unknown setting %q", args[0])
	}
	c.Arg = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *UnitGetCommand) Run(ctx *cmd.Context) (err error) {
	var unit *state.Unit
	unit, err = c.ctx.State.Unit(c.ctx.LocalUnitName)
	if err != nil {
		return
	}
	var value string
	if c.Arg == "private-address" {
		value, err = unit.PrivateAddress()
	} else {
		value, err = unit.PublicAddress()
	}
	if err != nil {
		return
	}
	return c.out.write(ctx, value)
}
