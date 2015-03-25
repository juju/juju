// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"gopkg.in/natefinch/lumberjack.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/unit"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/network"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var agentLogger = loggo.GetLogger("juju.jujud")

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	agentcmd.AgentConf
	UnitName     string
	runner       worker.Runner
	setupLogging func(agent.Config) error
	logToStdErr  bool
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
	f.BoolVar(&a.logToStdErr, "log-to-stderr", false, "whether to log to standard error instead of log files")
}

// Init initializes the command for running.
func (a *UnitAgent) Init(args []string) error {
	if a.UnitName == "" {
		return cmdutil.RequiredError("unit-name")
	}
	if !names.IsValidUnit(a.UnitName) {
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant)
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
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return err
	}
	agentConfig := a.CurrentConfig()

	if !a.logToStdErr {
		filename := filepath.Join(agentConfig.LogDir(), agentConfig.Tag().String()+".log")

		log := &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    300, // megabytes
			MaxBackups: 2,
		}

		if err := cmdutil.SwitchProcessToRollingLogs(log); err != nil {
			return err
		}
	}
	agentLogger.Infof("unit agent %v start (%s [%s])", a.Tag().String(), version.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	network.InitializeFromConfig(agentConfig)
	a.runner.StartWorker("api", a.APIWorkers)
	err := cmdutil.AgentDone(logger, a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

func (a *UnitAgent) APIWorkers() (_ worker.Worker, err error) {
	engine := dependency.NewEngine(cmdutil.IsFatal, 3*time.Second, 50*time.Millisecond)
	for name, manifold := range unit.AgentManifolds(a) {
		if err := engine.Install(name, manifold); err != nil {
			if e := worker.Stop(engine); e != nil {
				logger.Errorf("failure while stoppping enngine after error: %v", e)
			}
			return nil, errors.Trace(err)
		}
	}
	return engine, nil
}

func (a *UnitAgent) Tag() names.Tag {
	return names.NewUnitTag(a.UnitName)
}
