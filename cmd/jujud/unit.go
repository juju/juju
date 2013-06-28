// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/tomb"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb     tomb.Tomb
	Conf     AgentConf
	UnitName string
	runner   *worker.Runner
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
	a.runner = worker.NewRunner(isFatal, moreImportant)
	return nil
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	a.runner.Kill()
	return a.tomb.Wait()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	defer a.tomb.Done()
	if err := a.Conf.read(a.Tag()); err != nil {
		return err
	}
	a.runner.StartWorker("toplevel", func() (worker.Worker, error) {
		// TODO(rog) go1.1: use method expression
		return a.Workers()
	})
	err := agentDone(a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

// Workers returns a worker that runs the unit agent workers.
func (a *UnitAgent) Workers() (worker.Worker, error) {
	st, entity, err := openState(a.Conf.Conf, a)
	if err != nil {
		return nil, err
	}
	unit := entity.(*state.Unit)
	dataDir := a.Conf.DataDir
	runner := worker.NewRunner(allFatal, moreImportant)
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return NewUpgrader(st, unit, dataDir), nil
	})
	runner.StartWorker("uniter", func() (worker.Worker, error) {
		return uniter.NewUniter(st, unit.Name(), dataDir), nil
	})
	return newCloseWorker(runner, st), nil
}

func (a *UnitAgent) Entity(st *state.State) (AgentState, error) {
	return st.Unit(a.UnitName)
}

func (a *UnitAgent) APIEntity(st *api.State) (AgentAPIState, error) {
	return nil, fmt.Errorf("not implemented yet")
}

func (a *UnitAgent) Tag() string {
	return state.UnitTag(a.UnitName)
}
