package jujuc

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// UnitGetCommand implements the unit-get command.
type UnitGetCommand struct {
	*HookContext
	Key string
	out cmd.Output
}

func NewUnitGetCommand(ctx *HookContext) (cmd.Command, error) {
	return &UnitGetCommand{HookContext: ctx}, nil
}

func (c *UnitGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"unit-get", "<setting>", "print public-address or private-address", "",
	}
}

func (c *UnitGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
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
	var value string
	if c.Key == "private-address" {
		value, err = c.Unit.PrivateAddress()
	} else {
		value, err = c.Unit.PublicAddress()
	}
	if err != nil {
		return
	}
	return c.out.Write(ctx, value)
}
