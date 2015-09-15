// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
)

const relationAddDoc = `
"relation-add" inserts a relation for a unit.
`

// RelationAddCommand implements the relation-add command.
type RelationAddCommand struct {
	cmd.CommandBase
	ctx Context

	Name      string
	Interface string
}

func NewRelationAddCommand(ctx Context) (cmd.Command, error) {
	c := &RelationAddCommand{ctx: ctx}
	return c, nil
}

func (c *RelationAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "relation-add",
		Args:    "<name> <interface>",
		Purpose: "add relation endpoint",
		Doc:     relationSetDoc,
	}
}

func (c *RelationAddCommand) Init(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("invalid arguments")
	}

	c.Name = args[0]
	c.Interface = args[1]
	return nil
}

func (c *RelationAddCommand) Run(ctx *cmd.Context) (err error) {
	c.ctx.AddCharmRelation(c.Name, c.Interface)
	return nil
}
