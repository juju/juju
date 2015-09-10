// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// RelationIdsCommand implements the relation-ids command.
type RelationIdsCommand struct {
	cmd.CommandBase
	ctx  Context
	Name string
	out  cmd.Output
}

func NewRelationIdsCommand(ctx Context) (cmd.Command, error) {
	name := ""
	if r, err := ctx.HookRelation(); err == nil {
		name = r.Name()
	} else if cause := errors.Cause(err); !errors.IsNotFound(cause) {
		return nil, errors.Trace(err)
	}

	return &RelationIdsCommand{ctx: ctx, Name: name}, nil
}

func (c *RelationIdsCommand) Info() *cmd.Info {
	args := "<name>"
	doc := ""
	if r, err := c.ctx.HookRelation(); err == nil {
		// There's not much we can do about this error here.
		args = "[<name>]"
		doc = fmt.Sprintf("Current default relation name is %q.", r.Name())
	} else if !errors.IsNotFound(err) {
		logger.Errorf("Could not retrieve hook relation: %v", err)
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
	ids, err := c.ctx.RelationIds()
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	for _, id := range ids {
		r, err := c.ctx.Relation(id)
		if err == nil && r.Name() == c.Name {
			result = append(result, r.FakeId())
		} else if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	sort.Strings(result)
	return c.out.Write(ctx, result)
}
