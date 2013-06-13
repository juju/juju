// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
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
	return a.Conf.checkArgs(args)
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	a.tomb.Kill(nil)
	return a.tomb.Wait()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	if err := a.Conf.read(state.UnitTag(a.UnitName)); err != nil {
		return err
	}
	defer a.tomb.Done()
	err := RunAgentLoop(a.Conf.Conf, a)
	if ug, ok := err.(*UpgradeReadyError); ok {
		if err1 := ug.ChangeAgentTools(); err1 != nil {
			err = err1
			// Return and let upstart deal with the restart.
		}
	}
	return err
}

// RunOnce runs a unit agent once.
func (a *UnitAgent) RunOnce(st *state.State, e AgentState) error {
	unit := e.(*state.Unit)
	dataDir := a.Conf.DataDir
	tasks := []task{
		uniter.NewUniter(st, unit.Name(), dataDir),
		NewUpgrader(st, unit, dataDir),
	}
	if unit.IsPrincipal() {
		tasks = append(tasks,
			newDeployer(st, unit.WatchSubordinateUnits(), dataDir))
	}
	return runTasks(a.tomb.Dying(), tasks...)
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
