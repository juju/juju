// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"sort"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

// RelationIdsCommand implements the relation-ids command.
type RelationIdsCommand struct {
	cmd.CommandBase
	ctx  Context
	Name string
	out  cmd.Output
}

func NewRelationIdsCommand(ctx Context) cmd.Command {
	return &RelationIdsCommand{ctx: ctx}
}

func (c *RelationIdsCommand) Info() *cmd.Info {
	args := "<name>"
	doc := ""
	if r, found := c.ctx.HookRelation(); found {
		args = "[<name>]"
		doc = fmt.Sprintf("Current default relation name is %q.", r.Name())
	}
	return &cmd.Info{
		Name:    "relation-ids",
		Args:    args,
		Purpose: "list all relation ids with the given relation name",
		Doc:     doc,
	}
}

func (c *RelationIdsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *RelationIdsCommand) Init(args []string) error {
	if r, found := c.ctx.HookRelation(); found {
		c.Name = r.Name()
	}
	if len(args) > 0 {
		c.Name = args[0]
		args = args[1:]
	} else if c.Name == "" {
		return fmt.Errorf("no relation name specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationIdsCommand) Run(ctx *cmd.Context) error {
	result := []string{}
	for _, id := range c.ctx.RelationIds() {
		if r, found := c.ctx.Relation(id); found && r.Name() == c.Name {
			result = append(result, r.FakeId())
		}
	}
	sort.Strings(result)
	return c.out.Write(ctx, result)
}
