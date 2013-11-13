// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	workerlogger "launchpad.net/juju-core/worker/logger"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/upgrader"
)

var agentLogger = loggo.GetLogger("juju.jujud")

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb     tomb.Tomb
	Conf     AgentConf
	UnitName string
	runner   worker.Runner
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
	if !names.IsUnit(a.UnitName) {
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
	agentLogger.Infof("unit agent %v start", a.Tag())
	a.runner.StartWorker("api", a.APIWorkers)
	err := agentDone(a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

func (a *UnitAgent) APIWorkers() (worker.Worker, error) {
	agentConfig := a.Conf.config
	st, entity, err := openAPIState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	dataDir := a.Conf.dataDir
	runner := worker.NewRunner(connectionIsFatal(st), moreImportant)
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewUpgrader(st.Upgrader(), agentConfig), nil
	})
	runner.StartWorker("logger", func() (worker.Worker, error) {
		return workerlogger.NewLogger(st.Logger(), agentConfig), nil
	})
	runner.StartWorker("uniter", func() (worker.Worker, error) {
		return uniter.NewUniter(st.Uniter(), entity.Tag(), dataDir), nil
	})
	return newCloseWorker(runner, st), nil
}

func (a *UnitAgent) Entity(st *state.State) (AgentState, error) {
	return st.Unit(a.UnitName)
}

func (a *UnitAgent) Tag() string {
	return names.UnitTag(a.UnitName)
}
