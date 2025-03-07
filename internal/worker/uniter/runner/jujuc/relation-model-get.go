// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// RelationModelGetCommand implements the relation-model-get command.
type RelationModelGetCommand struct {
	cmd.CommandBase
	ctx             Context
	RelationId      int
	relationIdProxy gnuflag.Value
	out             cmd.Output
}

// NewRelationModelGetCommand creates a RelationModelGetCommand.
func NewRelationModelGetCommand(ctx Context) (cmd.Command, error) {
	c := &RelationModelGetCommand{ctx: ctx}

	rV, err := NewRelationIdValue(c.ctx, &c.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.relationIdProxy = rV

	return c, nil

}

// Info returns information about the Command.
func (c *RelationModelGetCommand) Info() *cmd.Info {
	doc := `
-r must be specified when not in a relation hook

relation-model-get outputs details about the model hosting the application
on the other end of a unit relation.
If not running in a relation hook context, -r needs to be specified with a
relation identifier similar to the relation-get and relation-set commands.
`
	if _, err := c.ctx.HookRelation(); err == nil {
		doc = ""
	}
	return jujucmd.Info(&cmd.Info{
		Name:    "relation-model-get",
		Purpose: "Get details about the model hosing a related application.",
		Doc:     doc,
	})
}

// SetFlags adds command specific flags to the flag set.
func (c *RelationModelGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.Var(c.relationIdProxy, "r", "Specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")
}

// Init initializes the Command before running.
func (c *RelationModelGetCommand) Init(args []string) (err error) {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	return cmd.CheckEmpty(args)
}

type modelDetails struct {
	UUID string `yaml:"uuid" json:"uuid"`
}

// Run executes the Command.
func (c *RelationModelGetCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}
	result := modelDetails{
		UUID: r.RemoteModelUUID(),
	}
	return c.out.Write(ctx, result)
}
