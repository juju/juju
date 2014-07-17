// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/state/api/params"
)

// ActionGetCommand implements the relation-get command.
type ActionGetCommand struct {
	cmd.CommandBase
	ctx        Context
	RelationId int
	Key        string
	UnitName   string
	out        cmd.Output
}

func NewActionGetCommand(ctx Context) cmd.Command {
	return &ActionGetCommand{ctx: ctx}
}

func (c *ActionGetCommand) Info() *cmd.Info {
	args := "<key>[.<key>.<key>...]"
	doc := `
action-get prints the values of the indicated parameter in the passed params
map.  If the value is a map, the values will be printed recursively as YAML.
`
	return &cmd.Info{
		Name:    "action-get",
		Args:    args,
		Purpose: "get relation settings",
		Doc:     doc,
	}
}

func (c *ActionGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(newRelationIdValue(c.ctx, &c.RelationId), "r", "specify a relation by id")
}

func (c *ActionGetCommand) Init(args []string) error {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	if name, found := c.ctx.RemoteUnitName(); found {
		c.UnitName = name
	}
	if len(args) > 0 {
		c.UnitName = args[0]
		args = args[1:]
	}
	if c.UnitName == "" {
		return fmt.Errorf("no unit id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *ActionGetCommand) Run(ctx *cmd.Context) error {
	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	var settings params.RelationSettings
	if c.UnitName == c.ctx.UnitName() {
		node, err := r.Settings()
		if err != nil {
			return err
		}
		settings = node.Map()
	} else {
		var err error
		settings, err = r.ReadSettings(c.UnitName)
		if err != nil {
			return err
		}
	}
	if c.Key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.Key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}
