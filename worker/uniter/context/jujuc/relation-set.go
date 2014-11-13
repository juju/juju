// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"
)

// RelationSetCommand implements the relation-set command.
type RelationSetCommand struct {
	cmd.CommandBase
	ctx        Context
	RelationId int
	Settings   map[string]string
	formatFlag string // deprecated
}

func NewRelationSetCommand(ctx Context) cmd.Command {
	return &RelationSetCommand{ctx: ctx}
}

func (c *RelationSetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "relation-set",
		Args:    "key=value [key=value ...]",
		Purpose: "set relation settings",
	}
}

func (c *RelationSetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(newRelationIdValue(c.ctx, &c.RelationId), "r", "specify a relation by id")
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *RelationSetCommand) Init(args []string) error {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	var err error
	c.Settings, err = keyvalues.Parse(args, true)
	return err
}

func (c *RelationSetCommand) Run(ctx *cmd.Context) (err error) {
	if c.formatFlag != "" {
		fmt.Fprintf(ctx.Stderr, "--format flag deprecated for command %q", c.Info().Name)
	}
	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	settings, err := r.Settings()
	if err != nil {
		return errors.Annotate(err, "cannot read relation settings")
	}
	for k, v := range c.Settings {
		if v != "" {
			settings.Set(k, v)
		} else {
			settings.Delete(k)
		}
	}
	return nil
}
