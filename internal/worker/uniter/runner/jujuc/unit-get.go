// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
)

// UnitGetCommand implements the unit-get command.
type UnitGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string
	out cmd.Output
}

func NewUnitGetCommand(ctx Context) (cmd.Command, error) {
	return &UnitGetCommand{ctx: ctx}, nil
}

func (c *UnitGetCommand) Info() *cmd.Info {
	doc := `
Further details:
unit-get returns the IP address of the unit.

It accepts a single argument, which must be
private-address or public-address. It is not
affected by context.

Note that if a unit has been deployed with
--bind space then the address returned from
unit-get private-address will get the address
from this space, not the ‘default’ space.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "unit-get",
		Args:    "<setting>",
		Purpose: "Print public-address or private-address.",
		Doc:     doc,
	})
}

func (c *UnitGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

func (c *UnitGetCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no setting specified")
	}
	if args[0] != "private-address" && args[0] != "public-address" {
		return fmt.Errorf("unknown setting %q", args[0])
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *UnitGetCommand) Run(ctx *cmd.Context) error {
	var value string
	var err error
	if c.Key == "private-address" {
		var networkInfos map[string]params.NetworkInfoResult
		networkInfos, err = c.ctx.NetworkInfo(ctx, []string{""}, -1)
		if err == nil {
			if networkInfos[""].Error != nil {
				err = errors.Trace(networkInfos[""].Error)
			}
		}
		// If we haven't found the address the NetworkInfo-way fall back to old, spaceless method
		if err != nil || len(networkInfos[""].Info) == 0 || len(networkInfos[""].Info[0].Addresses) == 0 {
			value, err = c.ctx.PrivateAddress()
		} else {
			// Here, we preserve behaviour that changed inadvertently in 2.8.7
			// when we pushed host name resolution from the network-get tool
			// into the NetworkInfo method on the uniter API.
			// If addresses were resolved from host names, return the host name
			// instead of the IP we resolved.
			addr := networkInfos[""].Info[0].Addresses[0]
			if addr.Hostname != "" {
				value = addr.Hostname
			} else {
				value = addr.Address
			}
		}
	} else {
		value, err = c.ctx.PublicAddress(ctx)
	}
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, value)
}
