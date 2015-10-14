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
	ctx             Context
	RelationId      int
	relationIdProxy gnuflag.Value
	out             cmd.Output
}

func NewRelationListCommand(ctx Context) (cmd.Command, error) {
	c := &RelationListCommand{ctx: ctx}

	rV, err := newRelationIdValue(c.ctx, &c.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.relationIdProxy = rV

	return c, nil

}

func (c *RelationListCommand) Info() *cmd.Info {
	doc := "-r must be specified when not in a relation hook"
	if _, err := c.ctx.HookRelation(); err == nil {
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
	f.Var(c.relationIdProxy, "r", "specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")
}

func (c *RelationListCommand) Init(args []string) (err error) {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationListCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}
	unitNames := r.UnitNames()
	if unitNames == nil {
		unitNames = []string{}
	}
	return c.out.Write(ctx, unitNames)
}
