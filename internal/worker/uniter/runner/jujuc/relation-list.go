// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// RelationListCommand implements the relation-list command.
type RelationListCommand struct {
	cmd.CommandBase
	ctx                   Context
	RelationId            int
	relationIdProxy       gnuflag.Value
	ListRemoteApplication bool
	out                   cmd.Output
}

func NewRelationListCommand(ctx Context) (cmd.Command, error) {
	c := &RelationListCommand{ctx: ctx}

	rV, err := NewRelationIdValue(c.ctx, &c.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.relationIdProxy = rV

	return c, nil

}

func (c *RelationListCommand) Info() *cmd.Info {
	doc := `
-r must be specified when not in a relation hook

relation-list outputs a list of all the related units for a relation identifier.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.
`
	if _, err := c.ctx.HookRelation(); err == nil {
		doc = ""
	}
	return jujucmd.Info(&cmd.Info{
		Name:    "relation-list",
		Purpose: "List relation units.",
		Doc:     doc,
	})
}

func (c *RelationListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.Var(c.relationIdProxy, "r", "Specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")
	f.BoolVar(&c.ListRemoteApplication, "app", false, "List remote application instead of participating units")
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

	if c.ListRemoteApplication {
		return c.out.Write(ctx, r.RemoteApplicationName())
	}

	unitNames := r.UnitNames()
	if unitNames == nil {
		unitNames = []string{}
	}
	return c.out.Write(ctx, unitNames)
}
