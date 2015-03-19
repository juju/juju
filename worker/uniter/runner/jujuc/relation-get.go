// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	cmd.CommandBase
	ctx        Context
	RelationId int
	Key        string
	UnitName   string
	out        cmd.Output
}

func NewRelationGetCommand(ctx Context) cmd.Command {
	return &RelationGetCommand{ctx: ctx}
}

func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit id>"
	doc := `
relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.
`
	if name, err := c.ctx.RemoteUnitName(); err == nil {
		args = "[<key> [<unit id>]]"
		doc += fmt.Sprintf("Current default unit id is %q.", name)
	}
	return &cmd.Info{
		Name:    "relation-get",
		Args:    args,
		Purpose: "get relation settings",
		Doc:     doc,
	}
}

func (c *RelationGetCommand) SetFlags(f *gnuflag.FlagSet) {
	rV := newRelationIdValue(c.ctx, &c.RelationId)

	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(rV, "r", "specify a relation by id")
	f.Var(rV, "relation", "")
}

func (c *RelationGetCommand) Init(args []string) error {
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
	if name, err := c.ctx.RemoteUnitName(); err == nil {
		c.UnitName = name
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
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

func (c *RelationGetCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if errors.IsNotFound(err) {
		return fmt.Errorf("unknown relation id")
	} else if err != nil {
		return errors.Trace(err)
	}
	var settings params.Settings
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
