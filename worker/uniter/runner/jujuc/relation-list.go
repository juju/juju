// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
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
	if _, ok := c.ctx.HookRelation(); ok {
		doc = ""
	}
	return &cmd.Info{
		Name:    "relation-list",
		Purpose: "list relation units",
		Doc:     doc,
	}
}

func (c *RelationListCommand) SetFlags(f *gnuflag.FlagSet) {
	rV := newRelationIdValue(c.ctx, &c.RelationId)

	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(rV, "r", "specify a relation by id")
	f.Var(rV, "relation", "")
}

func (c *RelationListCommand) Init(args []string) (err error) {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationListCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("unknown relation id")
	} else if err != nil {
		return errors.Trace(err)
	}
	unitNames := r.UnitNames()
	if unitNames == nil {
		unitNames = []string{}
	}
	return c.out.Write(ctx, unitNames)
}
