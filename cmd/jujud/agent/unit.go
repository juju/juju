// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/featureflag"
	"gopkg.in/natefinch/lumberjack.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/network"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/uniter"
)

var (
	agentLogger = loggo.GetLogger("juju.jujud")
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
	UnitName     string
	runner       worker.Runner
	bufferedLogs logsender.LogRecordCh
	setupLogging func(agent.Config) error
	logToStdErr  bool
	ctx          *cmd.Context

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	// Channel used as a selectable bool (closed means true).
	initialUpgradeCheckComplete chan struct{}
}

// NewUnitAgent creates a new UnitAgent value properly initialized.
func NewUnitAgent(ctx *cmd.Context, bufferedLogs logsender.LogRecordCh) *UnitAgent {
	return &UnitAgent{
		AgentConf: NewAgentConf(""),
		ctx:       ctx,
		initialUpgradeCheckComplete: make(chan struct{}),
		bufferedLogs:                bufferedLogs,
	}
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
	a.runner = worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant, worker.RestartDelay)

	if !a.logToStdErr {
		if err := a.ReadConfig(a.Tag().String()); err != nil {
			return err
		}
		agentConfig := a.CurrentConfig()

		// the writer in ctx.stderr gets set as the loggo writer in github.com/juju/cmd/logging.go
		a.ctx.Stderr = &lumberjack.Logger{
			Filename:   agent.LogFilename(agentConfig),
			MaxSize:    300, // megabytes
			MaxBackups: 2,
		}

	}

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

	agentLogger.Infof("unit agent %v start (%s [%s])", a.Tag().String(), version.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}
	network.SetPreferIPv6(agentConfig.PreferIPv6())

	// Sometimes there are upgrade steps that are needed for each unit.
	// There are plans afoot to unify the unit and machine agents. When
	// this happens, there will be a simple helper function for the upgrade
	// steps to run something for each unit on the machine. Until then, we
	// need to have the uniter do it, as the overhead of getting a full
	// upgrade process in the unit agent out weights the current benefits.
	// So.. since the upgrade steps are all idempotent, we will just call
	// the upgrade steps when we start the uniter. To be clear, these
	// should move back to the upgrade package when we do unify the agents.
	runUpgrades(agentConfig.Tag(), agentConfig.DataDir())

	a.runner.StartWorker("api", a.APIWorkers)
	err := cmdutil.AgentDone(logger, a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

// runUpgrades is a temporary fix to deal with upgrade steps that need
// to be run for each unit. This function cannot fail. Errors in the
// upgrade steps are logged, but the uniter will attempt to continue.
// Worst case, we are no worse off than we are today, best case, things
// actually work properly. Only simple upgrade steps that don't use the API
// are available now. If we need really complex steps using the API, there
// should be significant steps to unify the agents first.
func runUpgrades(tag names.Tag, dataDir string) {
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		logger.Errorf("unit agent tag not a unit tag: %v", tag)
		return
	}
	if err := uniter.AddStoppedFieldToUniterState(unitTag, dataDir); err != nil {
		logger.Errorf("Upgrade step failed - add Stopped field to uniter state: %v", err)
	}
	if err := uniter.AddInstalledToUniterState(unitTag, dataDir); err != nil {
		logger.Errorf("Upgrade step failed - installed boolean needs to be set in the uniter local state: %v", err)
	}
}

// APIWorkers returns a dependency.Engine running the unit agent's responsibilities.
func (a *UnitAgent) APIWorkers() (worker.Worker, error) {
	manifolds := unit.Manifolds(unit.ManifoldsConfig{
		Agent:               agent.APIHostPortsSetter{a},
		LogSource:           a.bufferedLogs,
		LeadershipGuarantee: 30 * time.Second,
	})

	config := dependency.EngineConfig{
		IsFatal:     cmdutil.IsFatal,
		WorstError:  cmdutil.MoreImportantError,
		ErrorDelay:  3 * time.Second,
		BounceDelay: 10 * time.Millisecond,
	}
	engine, err := dependency.NewEngine(config)
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	return engine, nil
}

func (a *UnitAgent) Tag() names.Tag {
	return names.NewUnitTag(a.UnitName)
}
