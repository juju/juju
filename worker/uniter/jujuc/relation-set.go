// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
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
	return &RelationSetCommand{ctx: ctx, Settings: map[string]string{}}
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
	for _, kv := range args {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			return fmt.Errorf(`expected "key=value", got %q`, kv)
		}
		c.Settings[parts[0]] = parts[1]
	}
	return nil
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
	for k, v := range c.Settings {
		if v != "" {
			settings.Set(k, v)
		} else {
			settings.Delete(k)
		}
	}
	return nil
}
