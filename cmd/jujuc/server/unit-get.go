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
	*ClientContext
	Key string
	out output
}

func NewUnitGetCommand(ctx *ClientContext) (cmd.Command, error) {
	if err := ctx.check(); err != nil {
		return nil, err
	}
	return &UnitGetCommand{ClientContext: ctx}, nil
}

func (c *UnitGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"unit-get", "<setting>", "print public-address or private-address", "",
	}
}

func (c *UnitGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.addFlags(f, "yaml", defaultFormatters)
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
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *UnitGetCommand) Run(ctx *cmd.Context) (err error) {
	var unit *state.Unit
	unit, err = c.State.Unit(c.LocalUnitName)
	if err != nil {
		return
	}
	var value string
	if c.Key == "private-address" {
		value, err = unit.PrivateAddress()
	} else {
		value, err = unit.PublicAddress()
	}
	if err != nil {
		return
	}
	if c.out.testMode {
		return truthError(value)
	}
	return c.out.write(ctx, value)
}
