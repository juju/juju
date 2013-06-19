// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujud/tasks"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/tomb"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb     tomb.Tomb
	Conf     AgentConf
	UnitName string
	runner   *tasks.Runner
}

// Info returns usage information for the command.
func (a *UnitAgent) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unit",
		Purpose: "run a juju unit agent",
	}
}

func (a *UnitAgent) SetFlags(f *gnuflag.FlagSet) {
	a.Conf.addFlags(f)
	f.StringVar(&a.UnitName, "unit-name", "", "name of the unit to run")
}

// Init initializes the command for running.
func (a *UnitAgent) Init(args []string) error {
	if a.UnitName == "" {
		return requiredError("unit-name")
	}
	if !state.IsUnitName(a.UnitName) {
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	if err := a.Conf.checkArgs(args); err != nil {
		return err
	}
	a.runner = tasks.NewRunner(isFatal, moreImportant)
	return nil
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	return a.runner.Stop()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	if err := a.Conf.read(state.Tag()); err != nil {
		return err
	}
	a.runner.StartTask(a.Tasks)
	return agentDone(a.runner.Wait())
}

// Tasks returns a Runner running the unit agent tasks.
func (a *UnitAgent) Tasks() (tasks.Task, error) {
	st, entity, err := openState(a.Conf.Conf, a)
	if err != nil {
		return nil, err
	}
	unit := entity.(*state.Unit)
	runner := tasks.NewRunner(allFatal, moreImportant),
		runner.StartTask("upgrader", func() (tasks.Task, error) {
			return NewUpgrader(st, unit, a.Conf.DataDir), nil
		})
	runner.StartTask("uniter", func() (tasks.Task, error) {
		return uniter.NewUniter(st, unit.Name(), a.Conf.DataDir)
	})
	return newCloseTask(runner, st), nil
 }

func (a *UnitAgent) Entity(st *state.State) (AgentState, error) {
	return st.Unit(a.UnitName)
}

func (a *UnitAgent) Tag() string {
	return state.UnitTag(a.UnitName)
}

func (a *UnitAgent) Tomb() *tomb.Tomb {
	return &a.tomb
}
