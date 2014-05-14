// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

// RelationListCommand implements the relation-list command.
type RelationListCommand struct {
	cmd.CommandBase
	ctx        Context
	RelationId int
	out        cmd.Output
}

func NewRelationListCommand(ctx Context) cmd.Command {
	return &RelationListCommand{ctx: ctx}
}

func (c *RelationListCommand) Info() *cmd.Info {
	doc := "-r must be specified when not in a relation hook"
	if _, found := c.ctx.HookRelation(); found {
		doc = ""
	}
	return &cmd.Info{
		Name:    "relation-list",
		Purpose: "list relation units",
		Doc:     doc,
	}
}

func (c *RelationListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(newRelationIdValue(c.ctx, &c.RelationId), "r", "specify a relation by id")
}

func (c *RelationListCommand) Init(args []string) (err error) {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationListCommand) Run(ctx *cmd.Context) error {
	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	unitNames := r.UnitNames()
	if unitNames == nil {
		unitNames = []string{}
	}
	return c.out.Write(ctx, unitNames)
}
