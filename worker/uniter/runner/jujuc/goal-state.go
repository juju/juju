// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
)

// GoalStateCommand implements the config-get command.
type GoalStateCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show. If empty, show all.
	out cmd.Output
}

func NewGoalStateCommand(ctx Context) (cmd.Command, error) {
	return &GoalStateCommand{ctx: ctx}, nil
}

func (c *GoalStateCommand) Info() *cmd.Info {
	doc := `
'goal-state' command will list the charm units and relations, specifying their status and their relations to other units in different charms.
`
	return &cmd.Info{
		Name:    "goal-state",
		Purpose: "print the status of the charm's peers and related units",
		Doc:     doc,
	}
}

func (c *GoalStateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
									"yaml":  cmd.FormatYaml,
									"json":  cmd.FormatJson,})
}

func (c *GoalStateCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = args[0]

	return cmd.CheckEmpty(args[1:])
}

func (c *GoalStateCommand) Run(ctx *cmd.Context) error {
	state, err := c.ctx.GoalState()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, state)
}