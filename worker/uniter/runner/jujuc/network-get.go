// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// NetworkGetCommand implements the network-get command.
type NetworkGetCommand struct {
	cmd.CommandBase
	ctx Context

	RelationId      int
	relationIdProxy gnuflag.Value
	primaryAddress  bool

	out cmd.Output
}

func NewNetworkGetCommand(ctx Context) (cmd.Command, error) {
	var err error
	cmd := &NetworkGetCommand{ctx: ctx}
	cmd.relationIdProxy, err = newRelationIdValue(ctx, &cmd.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *NetworkGetCommand) Info() *cmd.Info {
	args := "--primary-address"
	doc := `
network-get returns the network config for a relation. The only supported
flag for now is --primary-address, which is required and returns the IP
address the local unit should advertise as its endpoint to its peers.
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
	f.Var(c.relationIdProxy, "r", "specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")
	f.BoolVar(&c.primaryAddress, "primary-address", false, "get the primary address for the relation")
}

// Init is part of the cmd.Command interface.
func (c *NetworkGetCommand) Init(args []string) error {

	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}

	if !c.primaryAddress {
		return fmt.Errorf("--primary-address is currently required")
	}

	return cmd.CheckEmpty(args)
}

func (c *NetworkGetCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}

	netconfig, err := r.NetworkConfig()
	if err != nil {
		return err
	}
	if len(netconfig) < 1 {
		return fmt.Errorf("no network config available")
	}

	if c.primaryAddress {
		return c.out.Write(ctx, netconfig[0].Address)
	}
	return c.out.Write(ctx, nil)
}
