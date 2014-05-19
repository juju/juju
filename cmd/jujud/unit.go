// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"runtime"

	"github.com/juju/loggo"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/apiaddressupdater"
	workerlogger "launchpad.net/juju-core/worker/logger"
	"launchpad.net/juju-core/worker/rsyslog"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/upgrader"
)

var agentLogger = loggo.GetLogger("juju.jujud")

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
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
	a.AgentConf.AddFlags(f)
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
	if err := a.AgentConf.CheckArgs(args); err != nil {
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
	if err := a.ReadConfig(a.Tag()); err != nil {
		return err
	}
	agentLogger.Infof("unit agent %v start (%s [%s])", a.Tag(), version.Current, runtime.Compiler)
	a.runner.StartWorker("api", a.APIWorkers)
	err := agentDone(a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

func (a *UnitAgent) APIWorkers() (worker.Worker, error) {
	agentConfig := a.CurrentConfig()
	dataDir := agentConfig.DataDir()
	hookLock, err := hookExecutionLock(dataDir)
	if err != nil {
		return nil, err
	}
	st, entity, err := openAPIState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	runner := worker.NewRunner(connectionIsFatal(st), moreImportant)
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewUpgrader(st.Upgrader(), agentConfig), nil
	})
	runner.StartWorker("logger", func() (worker.Worker, error) {
		return workerlogger.NewLogger(st.Logger(), agentConfig), nil
	})
	runner.StartWorker("uniter", func() (worker.Worker, error) {
		return uniter.NewUniter(st.Uniter(), entity.Tag(), dataDir, hookLock), nil
	})
	runner.StartWorker("apiaddressupdater", func() (worker.Worker, error) {
		return apiaddressupdater.NewAPIAddressUpdater(st.Uniter(), a), nil
	})
	runner.StartWorker("rsyslog", func() (worker.Worker, error) {
		return newRsyslogConfigWorker(st.Rsyslog(), agentConfig, rsyslog.RsyslogModeForwarding)
	})
	return newCloseWorker(runner, st), nil
}

func (a *UnitAgent) Tag() string {
	return names.UnitTag(a.UnitName)
}
