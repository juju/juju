// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/lumberjack"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3/arch"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/core/machinelock"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/paths"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
)

// UnitAgent wraps the agent config for this unit.
type UnitAgent struct {
	tag    names.UnitTag
	name   string
	clock  clock.Clock
	logger Logger

	mu               sync.Mutex
	agentConf        agent.ConfigSetterWriter
	configChangedVal *voyeur.Value

	setupLogging       func(*loggo.Context, agent.Config)
	unitEngineConfig   func() dependency.EngineConfig
	unitManifolds      func(UnitManifoldsConfig) dependency.Manifolds
	prometheusRegistry *prometheus.Registry

	// Able to disable running units.
	workerRunning bool
}

// UnitAgentConfig is a params struct with the values necessary to
// construct a working unit agent.
type UnitAgentConfig struct {
	Name             string
	DataDir          string
	Clock            clock.Clock
	Logger           Logger
	UnitEngineConfig func() dependency.EngineConfig
	UnitManifolds    func(UnitManifoldsConfig) dependency.Manifolds
	SetupLogging     func(*loggo.Context, agent.Config)
}

// Validate ensures all the required values are set.
func (u *UnitAgentConfig) Validate() error {
	if u.Name == "" {
		return errors.NotValidf("missing Name")
	}
	if u.DataDir == "" {
		return errors.NotValidf("missing DataDir")
	}
	if u.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if u.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if u.SetupLogging == nil {
		return errors.NotValidf("missing SetupLogging")
	}
	if u.UnitEngineConfig == nil {
		return errors.NotValidf("missing UnitEngineConfig")
	}
	if u.UnitManifolds == nil {
		return errors.NotValidf("missing UnitManifolds")
	}
	return nil
}

// NewUnitAgent constructs an "agent" that is responsible for
// defining the workers for the unit and wraps access and updates
// to the agent.conf file for the unit. The method expects that there
// is an agent.conf file written in the <datadir>/agents/unit-<name>
// directory. It would be good to remove this need moving forwards
// and have unit agent logging overrides allowable in the machine
// agent config file.
func NewUnitAgent(config UnitAgentConfig) (*UnitAgent, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	// Create a symlink for the unit "agent" binaries.
	// This is used because the uniter is still using the tools directory
	// for the unit agent for creating the jujuc symlinks.
	config.Logger.Tracef("creating symlink for %q to tools directory for jujuc", config.Name)
	current := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: coreos.HostOSTypeName(),
	}
	tag := names.NewUnitTag(config.Name)
	toolsDir := tools.ToolsDir(config.DataDir, tag.String())
	_, err := tools.ChangeAgentTools(config.DataDir, tag.String(), current)
	defer removeOnErr(&err, config.Logger, toolsDir)
	if err != nil {
		// Any error here is indicative of a disk issue, potentially out of
		// space or inodes. Either way, bouncing the deployer and having the
		// exponential backoff enter play is the right decision.
		return nil, errors.Trace(err)
	}

	config.Logger.Infof("creating new agent config for %q", config.Name)
	conf, err := agent.ReadConfig(agent.ConfigPath(config.DataDir, tag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	prometheusRegistry, err := addons.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit := &UnitAgent{
		tag:                tag,
		name:               config.Name,
		clock:              config.Clock,
		logger:             config.Logger,
		agentConf:          conf,
		configChangedVal:   voyeur.NewValue(true),
		setupLogging:       config.SetupLogging,
		unitEngineConfig:   config.UnitEngineConfig,
		unitManifolds:      config.UnitManifolds,
		prometheusRegistry: prometheusRegistry,
	}
	// Update the 'upgradedToVersion' in the agent.conf file if it is
	// different to the current version.
	if conf.UpgradedToVersion() != jujuversion.Current {
		if err := unit.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetUpgradedToVersion(jujuversion.Current)
			return nil
		}); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return unit, nil
}

func (a *UnitAgent) start() (worker.Worker, error) {
	a.logger.Tracef("starting workers for %q", a.name)
	loggingContext, bufferedLogger, err := a.initLogging()
	if err != nil {
		a.logger.Tracef("init logging failed %s", err)
		return nil, errors.Trace(err)
	}

	updateAgentConfLogging := func(loggingConfig string) error {
		return a.ChangeConfig(func(setter agent.ConfigSetter) error {
			setter.SetLoggingConfig(loggingConfig)
			return nil
		})
	}

	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   a.tag.String(),
		Clock:       a.clock,
		Logger:      loggingContext.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(a.agentConf),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		a.logger.Tracef("creating machine lock failed %s", err)
		return nil, errors.Trace(err)
	}

	// construct unit agent manifold
	a.logger.Tracef("creating unit manifolds for %q", a.name)
	manifolds := a.unitManifolds(UnitManifoldsConfig{
		LoggingContext:      loggingContext,
		Agent:               a,
		LogSource:           bufferedLogger.Logs(),
		LeadershipGuarantee: 30 * time.Second,
		AgentConfigChanged:  a.configChangedVal,
		ValidateMigration:   a.validateMigration,
		UpdateLoggerConfig:  updateAgentConfLogging,
		MachineLock:         machineLock,
		Clock:               a.clock,
	})
	depEngineConfig := a.unitEngineConfig()
	// TODO: tweak IsFatal error func, maybe?
	depEngineConfig.Logger = loggingContext.GetLogger("juju.worker.dependency")
	// Tweak as necessary.
	engine, err := dependency.NewEngine(depEngineConfig)
	if err != nil {
		return nil, err
	}

	a.logger.Tracef("installing manifolds for %q", a.name)
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			a.logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, err
	}
	a.mu.Lock()
	a.workerRunning = true
	a.mu.Unlock()
	go func() {
		// Wait for the worker to finish, then mark not running.
		_ = engine.Wait()
		a.mu.Lock()
		a.workerRunning = false
		a.mu.Unlock()
	}()
	if err := addons.StartIntrospection(addons.IntrospectionConfig{
		AgentTag:           a.CurrentConfig().Tag(),
		Engine:             engine,
		NewSocketName:      addons.DefaultIntrospectionSocketName,
		PrometheusGatherer: a.prometheusRegistry,
		MachineLock:        machineLock,
		WorkerFunc:         introspection.NewWorker,
	}); err != nil {
		// If the introspection worker failed to start, we just log error
		// but continue. It is very unlikely to happen in the real world
		// as the only issue is connecting to the abstract domain socket
		// and the agent is controlled by by the OS to only have one.
		a.logger.Errorf("failed to start introspection worker: %v", err)
	}
	a.logger.Tracef("engine for %q running", a.name)
	return engine, nil
}

func (a *UnitAgent) running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.workerRunning
}

func (a *UnitAgent) initLogging() (*loggo.Context, *logsender.BufferedLogWriter, error) {
	loggingContext := loggo.NewContext(loggo.INFO)

	logFilename := agent.LogFilename(a.agentConf)
	if err := paths.PrimeLogFile(logFilename); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		a.logger.Errorf("unable to prime %s (proceeding anyway): %v", logFilename, err)
	}
	ljLogger := &lumberjack.Logger{
		Filename:   logFilename, // eg: "/var/log/juju/unit-mysql-0.log"
		MaxSize:    a.CurrentConfig().AgentLogfileMaxSizeMB(),
		MaxBackups: a.CurrentConfig().AgentLogfileMaxBackups(),
		Compress:   true,
	}
	a.logger.Debugf("created rotating log file %q with max size %d MB and max backups %d",
		ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
	if err := loggingContext.AddWriter(
		"file", loggo.NewSimpleWriter(ljLogger, loggo.DefaultFormatter)); err != nil {
		a.logger.Errorf("unable to configure file logging for unit %q: %v", a.name, err)
	}

	bufferedLogger, err := logsender.InstallBufferedLogWriter(loggingContext, 1048576)
	if err != nil {
		return nil, nil, errors.Annotate(err, "unable to add buffered log writer")
	}
	// Add line for starting agent to logging context.
	loggingContext.GetLogger("juju").Infof("Starting unit workers for %q", a.name)
	a.setupLogging(loggingContext, a.agentConf)
	return loggingContext, bufferedLogger, nil
}

// ChangeConfig modifies this configuration using the given mutator.
func (a *UnitAgent) ChangeConfig(change agent.ConfigMutator) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := change(a.agentConf); err != nil {
		return errors.Trace(err)
	}
	if err := a.agentConf.Write(); err != nil {
		return errors.Annotate(err, "cannot write agent configuration")
	}
	a.configChangedVal.Set(true)
	return nil
}

// CurrentConfig returns the agent config for this agent.
func (a *UnitAgent) CurrentConfig() agent.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agentConf.Clone()
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *UnitAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	facade := uniter.NewState(apiCaller, a.tag)
	_, err := facade.Unit(a.tag)
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
