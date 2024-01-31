// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/application"
)

// GoalStateCommand implements the config-get command.
type GoalStateCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

func NewGoalStateCommand(ctx Context) (cmd.Command, error) {
	return &GoalStateCommand{ctx: ctx}, nil
}

func (c *GoalStateCommand) Info() *cmd.Info {
	doc := `
'goal-state' command will list the charm units and relations, specifying their status and their relations to other units in different charms.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "goal-state",
		Purpose: "print the status of the charm's peers and related units",
		Doc:     doc,
	})
}

func (c *GoalStateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson})
}

func (c *GoalStateCommand) Init(args []string) error {
	return cmd.CheckEmpty(args[:])
}

func (c *GoalStateCommand) Run(ctx *cmd.Context) error {
	goalState, err := c.ctx.GoalState(ctx)
	if err != nil {
		return err
	}

	goalStateFormated := formatGoalState(*goalState)
	return c.out.Write(ctx, goalStateFormated)
}

// goalStateStatusContents is used to format application.GoalState.Since
// using strings.
type goalStateStatusContents struct {
	Status string `json:"status" yaml:"status"`
	Since  string `json:"since,omitempty" yaml:"since,omitempty"`
}

// unitsGoalStateContents keeps the collection of units and their GoalStateStatus.
type unitsGoalStateContents map[string]goalStateStatusContents

// formattedGoalState is responsible to organize the Units and Relations with a specific
// unit, and transmit this information from the facade to the worker.
type formattedGoalState struct {
	Units     unitsGoalStateContents            `json:"units" yaml:"units"`
	Relations map[string]unitsGoalStateContents `json:"relations" yaml:"relations"`
}

// formatGoalState moves information from application GoalState struct to
// application GoalState struct.
func formatGoalState(gs application.GoalState) formattedGoalState {
	result := formattedGoalState{}

	copyUnits := func(units application.UnitsGoalState) unitsGoalStateContents {
		copiedUnits := unitsGoalStateContents{}
		for name, gs := range units {
			copiedUnits[name] = goalStateStatusContents{
				Status: gs.Status,
				Since:  common.FormatTime(gs.Since, true),
			}
		}
		return copiedUnits
	}

	result.Units = copyUnits(gs.Units)
	result.Relations = make(map[string]unitsGoalStateContents, len(gs.Relations))
	for relation, units := range gs.Relations {
		result.Relations[relation] = copyUnits(units)
	}

	return result
}
