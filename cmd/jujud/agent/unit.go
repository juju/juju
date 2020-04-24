// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/upgrades"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/upgradesteps"
)

var (
	// should be an explicit dependency, can't do it cleanly yet
	unitManifolds = unit.Manifolds
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	cmd.CommandBase
	AgentConf
	configChangedVal *voyeur.Value
	UnitName         string
	runner           *worker.Runner
	bufferedLogger   *logsender.BufferedLogWriter
	setupLogging     func(agent.Config) error
	logToStdErr      bool
	ctx              *cmd.Context
	dead             chan struct{}
	errReason        error

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	initialUpgradeCheckComplete gate.Lock
	preUpgradeSteps             upgrades.PreUpgradeStepsFunc
	upgradeComplete             gate.Lock

	prometheusRegistry *prometheus.Registry
}

// NewUnitAgent creates a new UnitAgent value properly initialized.
func NewUnitAgent(ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (*UnitAgent, error) {
	prometheusRegistry, err := newPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UnitAgent{
		AgentConf:                   NewAgentConf(""),
		configChangedVal:            voyeur.NewValue(true),
		ctx:                         ctx,
		dead:                        make(chan struct{}),
		initialUpgradeCheckComplete: gate.NewLock(),
		bufferedLogger:              bufferedLogger,
		prometheusRegistry:          prometheusRegistry,
		preUpgradeSteps:             upgrades.PreUpgradeSteps,
	}, nil
}

// Info returns usage information for the command.
func (a *UnitAgent) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Purpose: "run a juju unit agent",
	})
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
		return errors.Errorf(`--unit-name option expects "<application>/<n>" argument`)
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = worker.NewRunner(worker.RunnerParams{
		IsFatal:       cmdutil.IsFatal,
		MoreImportant: cmdutil.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})

	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return err
	}
	agentConfig := a.CurrentConfig()

	if !a.logToStdErr {

		// the writer in ctx.stderr gets set as the loggo writer in github.com/juju/cmd/logging.go
		a.ctx.Stderr = &lumberjack.Logger{
			Filename:   agent.LogFilename(agentConfig),
			MaxSize:    300, // megabytes
			MaxBackups: 2,
			Compress:   true,
		}
	}
	return nil
}

// Stop stops the unit agent.
func (a *UnitAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Wait waits for the unit agent to finish
func (a *UnitAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Done signals the unit agent is finished
func (a *UnitAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

func (a *UnitAgent) isUpgradeRunning() bool {
	return !a.upgradeComplete.IsUnlocked()
}

func (a *UnitAgent) isInitialUpgradeCheckPending() bool {
	return !a.initialUpgradeCheckComplete.IsUnlocked()
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return err
	}
	setupAgentLogging(a.CurrentConfig())

	a.runner.StartWorker("api", a.APIWorkers)
	err = cmdutil.AgentDone(logger, a.runner.Wait())
	return err
}

// APIWorkers returns a dependency.Engine running the unit agent's responsibilities.
func (a *UnitAgent) APIWorkers() (worker.Worker, error) {
	updateAgentConfLogging := func(loggingConfig string) error {
		return a.AgentConf.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	agentConfig := a.AgentConf.CurrentConfig()
	a.upgradeComplete = upgradesteps.NewLock(agentConfig)
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   a.Tag().String(),
		Clock:       clock.WallClock,
		Logger:      loggo.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(agentConfig),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifolds := unitManifolds(unit.ManifoldsConfig{
		Agent:                agent.APIHostPortsSetter{a},
		LogSource:            a.bufferedLogger.Logs(),
		LeadershipGuarantee:  30 * time.Second,
		AgentConfigChanged:   a.configChangedVal,
		ValidateMigration:    a.validateMigration,
		PrometheusRegisterer: a.prometheusRegistry,
		UpdateLoggerConfig:   updateAgentConfLogging,
		PreviousAgentVersion: agentConfig.UpgradedToVersion(),
		PreUpgradeSteps:      a.preUpgradeSteps,
		UpgradeStepsLock:     a.upgradeComplete,
		UpgradeCheckLock:     a.initialUpgradeCheckComplete,
		MachineLock:          machineLock,
		Clock:                clock.WallClock,
	})

	engine, err := dependency.NewEngine(dependencyEngineConfig())
	if err != nil {
		return nil, err
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	if err := startIntrospection(introspectionConfig{
		Agent:              a,
		Engine:             engine,
		NewSocketName:      DefaultIntrospectionSocketName,
		PrometheusGatherer: a.prometheusRegistry,
		MachineLock:        machineLock,
		WorkerFunc:         introspection.NewWorker,
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		logger.Errorf("failed to start introspection worker: %v", err)
	}
	return engine, nil
}

func (a *UnitAgent) Tag() names.Tag {
	return names.NewUnitTag(a.UnitName)
}

func (a *UnitAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConf.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *UnitAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	unitTag := names.NewUnitTag(a.UnitName)
	facade := uniter.NewState(apiCaller, unitTag)
	_, err := facade.Unit(unitTag)
	if err != nil {
		return errors.Trace(err)
	}
	model, err := facade.Model()
	if err != nil {
		return errors.Trace(err)
	}
	curModelUUID := a.CurrentConfig().Model().Id()
	newModelUUID := model.UUID
	if newModelUUID != curModelUUID {
		return errors.Errorf("model mismatch when validating: got %q, expected %q",
			newModelUUID, curModelUUID)
	}
	return nil
}
