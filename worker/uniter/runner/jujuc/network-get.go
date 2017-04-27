// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
)

// NetworkGetCommand implements the network-get command.
type NetworkGetCommand struct {
	cmd.CommandBase
	ctx Context

	bindingName    string
	primaryAddress bool

	out cmd.Output
}

func NewNetworkGetCommand(ctx Context) (cmd.Command, error) {
	cmd := &NetworkGetCommand{ctx: ctx}
	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *NetworkGetCommand) Info() *cmd.Info {
	args := "<binding-name> --primary-address"
	doc := `
network-get returns the network config for a given binding name. The only
supported flag for now is --primary-address, which is required and returns
the IP address the local unit should advertise as its endpoint to its peers.
`
	return &cmd.Info{
		Name:    "network-get",
		Args:    args,
		Purpose: "get network config",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *NetworkGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.primaryAddress, "primary-address", false, "get the primary address for the binding")
}

// Init is part of the cmd.Command interface.
func (c *NetworkGetCommand) Init(args []string) error {

	if len(args) < 1 {
		return errors.New("no arguments specified")
	}
	c.bindingName = args[0]
	if c.bindingName == "" {
		return fmt.Errorf("no binding name specified")
	}

	if !c.primaryAddress {
		return fmt.Errorf("--primary-address is currently required")
	}

	return cmd.CheckEmpty(args[1:])
}

func (c *NetworkGetCommand) Run(ctx *cmd.Context) error {
	netInfo, err := c.ctx.NetworkInfo([]string{c.bindingName})
	if err != nil {
		return errors.Trace(err)
	}

	ni, ok := netInfo[c.bindingName]

	if !ok {
		return fmt.Errorf("no network config found for binding %q", c.bindingName)
	}
	if ni.Error != nil {
		return errors.Trace(ni.Error)
	}

	if c.primaryAddress {
		if len(ni.Info[0].Addresses) == 0 {
			return fmt.Errorf("No addresses attached to space for binding %q", c.bindingName)
		}
		return c.out.Write(ctx, ni.Info[0].Addresses[0])
	}

	return nil // never reached as --primary-address is required.
}
