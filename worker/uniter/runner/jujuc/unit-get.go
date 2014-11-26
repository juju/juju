// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"errors"
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

const unitGetDoc = `
"unit-get" is used in charm hooks to get information which is otherwise
unavailable about the unit for the currently executing hook. To get the
information specify the corresponding key when running "unit-get".

Accepted keys:
 public-address - the unit's public IP address as exposed by juju
 private-address - the unit's juju-internal IP address, accessible to
    other units
 availability-zone - the availability zone (if any) in which the machine
    containing the unit was provisioned

Note that for availability zones, not all providers support them. In the
case that a provider does not, an empty string ("") will be returned.
`

var unitgetKeys = []string{
	"private-address",
	"public-address",
	"availability-zone",
}

// UnitGetCommand implements the unit-get command.
type UnitGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string
	out cmd.Output
}

func NewUnitGetCommand(ctx Context) cmd.Command {
	return &UnitGetCommand{ctx: ctx}
}

func (c *UnitGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unit-get",
		Args:    "<setting>",
		Purpose: `print availability-zone, public-address, or private-address`,
		Doc:     unitGetDoc,
	}
}

func (c *UnitGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *UnitGetCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no setting specified")
	}
	for _, key := range unitgetKeys {
		if args[0] == key {
			c.Key = key
			return cmd.CheckEmpty(args[1:])
		}
	}
	return fmt.Errorf("unknown setting %q", args[0])
}

func (c *UnitGetCommand) Run(ctx *cmd.Context) error {
	value, ok := "", false

	switch c.Key {
	case "private-address":
		value, ok = c.ctx.PrivateAddress()
	case "public-address":
		value, ok = c.ctx.PublicAddress()
	case "availability-zone":
		value, ok = c.ctx.AvailabilityZone()
	}
	if !ok {
		return fmt.Errorf("%s not set", c.Key)
	}

	return c.out.Write(ctx, value)
}
