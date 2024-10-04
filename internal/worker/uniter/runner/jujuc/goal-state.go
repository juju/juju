// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
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
'goal-state' command will list the charm units and relations, specifying their status and
their relations to other units in different charms.
goal-state queries information about charm deployment and returns it as structured data.

goal-state provides:
    - the details of other peer units have been deployed and their status
    - the details of remote units on the other end of each endpoint and their status

The output will be a subset of that produced by the juju status. There will be output
for sibling (peer) units and relation state per unit.

The unit status values are the workload status of the (sibling) peer units. We also use
a unit status value of dying when the unit’s life becomes dying. Thus unit status is one of:
    - allocating
    - active
    - waiting
    - blocked
    - error
    - dying

The relation status values are determined per unit and depend on whether the unit has entered
or left scope. The possible values are:
    - joining : a relation has been created, but no units are available. This occurs when the
      application on the other side of the relation is added to a model, but the machine hosting
      the first unit has not yet been provisioned. Calling relation-set will work correctly as
      that data will be passed through to the unit when it comes online, but relation-get will
      not provide any data.
    - joined : the relation is active. A unit has entered scope and is accessible to this one.
    - broken : unit has left, or is preparing to leave scope. Calling relation-get is not advised
      as the data will quickly out of date when the unit leaves.
    - suspended : parent cross model relation is suspended
    - error: an external error has been detected

By reporting error state, the charm has a chance to determine that goal state may not be reached
due to some external cause. As with status, we will report the time since the status changed to
allow the charm to empirically guess that a peer may have become stuck if it has not yet reached
active state.
`
	examples := `
    goal-state
`
	return jujucmd.Info(&cmd.Info{
		Name:     "goal-state",
		Purpose:  "Print the status of the charm's peers and related units.",
		Doc:      doc,
		Examples: examples,
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
