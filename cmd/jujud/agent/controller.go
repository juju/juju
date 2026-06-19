// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v3"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	agentconfig "github.com/juju/juju/agent/config"
	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	coreapiserver "github.com/juju/juju/apiserver"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	agentcontroller "github.com/juju/juju/cmd/jujud/agent/controller"
	"github.com/juju/juju/cmd/jujud/agent/model"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	internaldependency "github.com/juju/juju/internal/dependency"
	"github.com/juju/juju/internal/flightrecorder"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/service"
	internalupgrade "github.com/juju/juju/internal/upgrade"
	"github.com/juju/juju/internal/upgradesteps"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apiserver"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
	"github.com/juju/juju/internal/worker/dbaccessor"
	workerflightrecorder "github.com/juju/juju/internal/worker/flightrecorder"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/identityfilewriter"
	"github.com/juju/juju/internal/worker/introspection"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/internal/wrench"
)

// controllerStartupValueProvider supplies current controller-local startup
// values to workers when they start. It re-reads runtime.conf and current
// agent config on each call so bounced workers do not keep stale values.
// It implements both ControllerStartupValueProvider (for controller
// manifolds) and model.StartupValueProvider (for model manifolds).
type controllerStartupValueProvider struct {
	agent                 *ControllerAgent
	controllerRuntimePath string
}

// ControllerStartupValues returns the current controller-local dbaccessor
// startup values from runtime.conf.
func (p controllerStartupValueProvider) ControllerStartupValues() (dbaccessor.ControllerStartupValues, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return dbaccessor.ControllerStartupValues{}, errors.Trace(err)
	}
	return dbaccessor.ControllerStartupValues{
		ControllerID:          cfg.ControllerID,
		DataDir:               cfg.DataDir,
		DqlitePort:            cfg.DqlitePort,
		QueryTracingEnabled:   cfg.QueryTracingEnabled,
		QueryTracingThreshold: cfg.QueryTracingThreshold,
		DqliteBusyTimeout:     cfg.DqliteBusyTimeout,
		CACert:                cfg.CACert,
		ControllerCert:        cfg.ControllerCert,
		ControllerPrivateKey:  cfg.ControllerPrivateKey,
	}, nil
}

// readRuntimeConfig returns the current controller runtime config from disk.
func (p controllerStartupValueProvider) readRuntimeConfig() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
	return controllerruntimeconfig.ReadControllerRuntimeConfig(p.controllerRuntimePath)
}

// CertMaterial returns the current controller certificate material.
func (p controllerStartupValueProvider) CertMaterial() (apiservercertwatcher.CertMaterial, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return apiservercertwatcher.CertMaterial{}, errors.Trace(err)
	}
	return apiservercertwatcher.CertMaterial{
		CACert:               cfg.CACert,
		CAPrivateKey:         cfg.CAPrivateKey,
		ControllerCert:       cfg.ControllerCert,
		ControllerPrivateKey: cfg.ControllerPrivateKey,
	}, nil
}

// LocalValues returns the current controller-local API server values.
func (p controllerStartupValueProvider) LocalValues() (apiserver.LocalValues, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return apiserver.LocalValues{}, errors.Trace(err)
	}
	return apiserver.LocalValues{
		DataDir:       cfg.DataDir,
		LogDir:        cfg.LogDir,
		LogSinkConfig: logSinkConfigFromRuntimeConfig(cfg),
	}, nil
}

// ObjectStoreRootDir returns the current local root dir for file-backed object
// store workers.
func (p controllerStartupValueProvider) ObjectStoreRootDir() (string, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return cfg.DataDir, nil
}

// APIInfo returns the current API connection info from runtime.conf.
func (p controllerStartupValueProvider) APIInfo() (*api.Info, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &api.Info{
		Addrs:          cfg.APIAddresses,
		CACert:         cfg.CACert,
		ControllerUUID: cfg.ControllerUUID,
		Tag:            names.NewControllerAgentTag(cfg.ControllerID),
	}, nil
}

// LoggingOverride returns the current persisted logging override.
// If runtime.conf contains a logging override, it takes precedence;
// otherwise runtime.conf's logging config is returned.
func (p controllerStartupValueProvider) LoggingOverride() (string, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	if cfg.LoggingOverride != "" {
		return cfg.LoggingOverride, nil
	}
	return cfg.LoggingConfig, nil
}

// SystemIdentityValues returns the current system identity file contents and
// path used by the controller identity writer.
func (p controllerStartupValueProvider) SystemIdentityValues() (identityfilewriter.SystemIdentityValues, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return identityfilewriter.SystemIdentityValues{}, errors.Trace(err)
	}
	return identityfilewriter.SystemIdentityValues{
		SystemIdentity:     cfg.SystemIdentity,
		SystemIdentityPath: filepath.Join(cfg.DataDir, agent.SystemIdentity),
	}, nil
}

// CACert returns the CA certificate from runtime.conf.
func (p controllerStartupValueProvider) CACert() (string, error) {
	cfg, err := p.readRuntimeConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return cfg.CACert, nil
}

type controllerAgentFactoryFnType func(names.Tag) (*ControllerAgent, error)

// NewControllerAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// ControllerAgent.
func NewControllerAgentCommand(
	ctx *cmd.Context,
	controllerAgentFactory controllerAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &controllerAgentCommand{
		ctx:                    ctx,
		controllerAgentFactory: controllerAgentFactory,
		agentInitializer:       agentInitializer,
		currentConfig:          configFetcher,
	}
}

type controllerAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer       AgentInitializer
	currentConfig          agentconfig.AgentConfigWriter
	controllerAgentFactory controllerAgentFactoryFnType
	ctx                    *cmd.Context

	// This group is for debugging purposes.
	logToStdErr bool

	agentTag              names.Tag
	controllerRuntimePath string

	// The following are set via command-line flags.
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *controllerAgentCommand) Init(args []string) error {
	if a.controllerId == "" {
		return errors.New("--controller-id must be set")
	}
	if !names.IsValidControllerAgent(a.controllerId) {
		return errors.Errorf("--controller-id option must be a non-negative integer")
	}
	if err := a.agentInitializer.CheckArgs(args); err != nil {
		return err
	}

	// Due to changes in the logging, and needing to care about old
	// models that have been upgraded, we need to explicitly remove
	// the file writer if one has been added, otherwise we will get
	// duplicate lines of all logging in the log file.
	_, _ = loggo.RemoveWriter("logfile")

	a.agentTag = names.NewControllerAgentTag(a.controllerId)
	a.controllerRuntimePath = controllerruntimeconfig.ConfigPath(filepath.Join(
		a.agentInitializer.DataDir(), "agents", "controller-"+a.agentTag.Id(),
	))
	if err := agentconfig.ReadAgentConfig(a.currentConfig, a.agentTag.Id()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}
	config := a.currentConfig.CurrentConfig()
	if err := os.MkdirAll(config.LogDir(), 0o644); err != nil {
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}

	if !a.logToStdErr {
		// the context's stderr is set as the loggo writer in
		// github.com/juju/juju/internal/cmd/logging.go
		ljLogger := &lumberjack.Logger{
			// eg: "/var/log/juju/controller-0.log"
			Filename:   agent.LogFilename(config),
			MaxSize:    config.AgentLogfileMaxSizeMB(),
			MaxBackups: config.AgentLogfileMaxBackups(),
			Compress:   true,
		}
		logger.Debugf(context.TODO(),
			"created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		a.ctx.Stderr = ljLogger
	}

	return nil
}

// Run instantiates a ControllerAgent and runs it.
func (a *controllerAgentCommand) Run(c *cmd.Context) error {
	controllerAgent, err := a.controllerAgentFactory(a.agentTag)
	if err != nil {
		return errors.Trace(err)
	}
	controllerAgent.controllerRuntimePath = a.controllerRuntimePath
	return controllerAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *controllerAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
	f.BoolVar(&a.logToStdErr, "log-to-stderr", false, "log to stderr instead of logsink.log")
}

// Info returns usage information for the command.
func (a *controllerAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "controller",
		Purpose: "run a juju controller agent",
	})
}

// ControllerAgentFactoryFn returns a function which instantiates a
// ControllerAgent given a controller agent tag.
func ControllerAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	preUpgradeSteps PreUpgradeStepsFunc,
	upgradeSteps UpgradeStepsFunc,
	rootDir string,
) controllerAgentFactoryFnType {
	return func(agentTag names.Tag) (*ControllerAgent, error) {
		runner, err := worker.NewRunner(worker.RunnerParams{
			Name:          "controller",
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  internalworker.RestartDelay,
			Logger:        internalworker.WrapLogger(logger),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return NewControllerAgent(
			agentTag,
			agentConfWriter,
			runner,
			newDBWorkerFunc,
			preUpgradeSteps,
			upgradeSteps,
			rootDir,
		)
	}
}

// NewControllerAgent instantiates a new ControllerAgent.
func NewControllerAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	runner *worker.Runner,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	preUpgradeSteps PreUpgradeStepsFunc,
	upgradeSteps UpgradeStepsFunc,
	rootDir string,
) (*ControllerAgent, error) {
	prometheusRegistry, err := addons.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	a := &ControllerAgent{
		agentTag:           agentTag,
		AgentConfigWriter:  agentConfWriter,
		configChangedVal:   voyeur.NewValue(true),
		workersStarted:     make(chan struct{}),
		dead:               make(chan struct{}),
		runner:             runner,
		rootDir:            rootDir,
		upgradeCheckLock:   gate.NewLock(),
		newDBWorkerFunc:    newDBWorkerFunc,
		prometheusRegistry: prometheusRegistry,
		preUpgradeSteps:    preUpgradeSteps,
		upgradeSteps:       upgradeSteps,
	}
	return a, nil
}

// ControllerAgent is responsible for tying together all functionality
// needed to orchestrate a jujud instance which controls a controller.
type ControllerAgent struct {
	agentconfig.AgentConfigWriter

	ctx                   *cmd.Context
	dead                  chan struct{}
	errReason             error
	agentTag              names.Tag
	runner                *worker.Runner
	rootDir               string
	configChangedVal      *voyeur.Value
	controllerRuntimePath string

	workersStarted chan struct{}

	newDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// upgradeCheckLock coordinates workers that should not start
	// until the upgrader worker has completed its first check.
	upgradeCheckLock gate.Lock

	prometheusRegistry *prometheus.Registry

	// To allow for testing in legacy tests (brittle integration tests),
	// we need to override these.
	preUpgradeSteps PreUpgradeStepsFunc
	upgradeSteps    UpgradeStepsFunc

	bootstrapLock         gate.Lock
	controllerUpgradeLock gate.Lock
	upgradeDBLock         gate.Waiter
	upgradeStepsLock      gate.Lock
}

// Wait waits for the controller agent to finish.
func (a *ControllerAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the controller agent.
func (a *ControllerAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the controller agent is finished.
func (a *ControllerAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// WorkersStarted returns a channel that's closed once all top level
// workers have been started. This is provided for testing purposes.
func (a *ControllerAgent) WorkersStarted() <-chan struct{} {
	return a.workersStarted
}

// Tag returns the controller agent's tag.
func (a *ControllerAgent) Tag() names.Tag {
	return a.agentTag
}

// ChangeConfig updates the agent's configuration and notifies
// listeners of the change.
func (a *ControllerAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

// Restart restarts the agent's service.
func (a *ControllerAgent) Restart() error {
	name := a.CurrentConfig().Value(agent.AgentServiceName)
	return service.Restart(name)
}

func (a *ControllerAgent) registerPrometheusCollectors() error {
	return nil
}

// Run runs a controller agent.
func (a *ControllerAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)
	a.ctx = ctx
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentconf.SetupAgentLogging(internallogger.DefaultContext(), a.CurrentConfig())
	agentConfig := a.CurrentConfig()
	controllerRuntimeConfig, err := controllerruntimeconfig.ReadControllerRuntimeConfig(a.controllerRuntimePath)
	if err != nil {
		return errors.Trace(err)
	}

	// Prime the log sink and create the writer.
	logSink, err := PrimeLogSink(agentConfig)
	if err != nil {
		return errors.Trace(err)
	}
	defer logSink.Close()

	// Add the log sink to the default logger context.
	if err := loggo.DefaultContext().AddWriter(
		"logsink", corelogger.NewTaggedRedirectWriter(
			logSink,
			a.Tag().String(),
			controllerRuntimeConfig.ControllerModelUUID,
		),
	); err != nil {
		return errors.Trace(err)
	}

	if err := introspection.WriteProfileFunctions(
		introspection.ProfileDir,
	); err != nil {
		// This isn't fatal, just annoying.
		logger.Errorf(context.Background(), "failed to write profile funcs: %v", err)
	}

	if err := a.registerPrometheusCollectors(); err != nil {
		return errors.Trace(err)
	}

	agentName := a.Tag().String()

	a.bootstrapLock = gate.NewLock()
	a.controllerUpgradeLock = gate.NewLock()
	a.upgradeDBLock = gate.AlreadyUnlocked{}
	a.upgradeStepsLock = internalupgrade.NewLock(agentConfig, jujuversion.Current)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion(), logSink)
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start.
	close(a.workersStarted)
	err = a.runner.Wait()
	return cmdutil.AgentDone(logger, err)
}

func (a *ControllerAgent) makeEngineCreator(
	agentName string,
	previousAgentVersion semversion.Number,
	logSink corelogger.LogSink,
) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		controllerRuntimeConfig, err := controllerruntimeconfig.ReadControllerRuntimeConfig(a.controllerRuntimePath)
		if err != nil {
			return nil, errors.Trace(err)
		}
		engineConfigFunc := agentengine.DependencyEngineConfig
		metrics := agentengine.NewMetrics()
		controllerMetricsSink := metrics.ForModel(names.NewModelTag(controllerRuntimeConfig.ControllerModelUUID))
		eng, err := dependency.NewEngine(engineConfigFunc(
			controllerMetricsSink,
			internaldependency.WrapLogger(internallogger.GetLogger(
				"juju.worker.dependency",
			)),
		))
		if err != nil {
			return nil, err
		}
		updateAgentConfLogging := func(loggingConfig string) error {
			return controllerruntimeconfig.ChangeControllerRuntimeConfig(
				a.controllerRuntimePath,
				func(cfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
					cfg.LoggingConfig = loggingConfig
					return nil
				},
			)
		}

		registerIntrospectionHandlers := func(handle func(path string, h http.Handler)) {
			handle("/metrics/", promhttp.HandlerFor(a.prometheusRegistry, promhttp.HandlerOpts{}))
		}

		c := clock.WallClock
		startupValueProvider := controllerStartupValueProvider{
			agent:                 a,
			controllerRuntimePath: a.controllerRuntimePath,
		}
		flightRecorder := workerflightrecorder.New(
			flightrecorder.NewRecorder(c), "",
			internallogger.GetLogger("juju.flightrecorder"),
		)

		manifoldsCfg := agentcontroller.ManifoldsConfig{
			PreviousAgentVersion: previousAgentVersion,
			AgentName:            agentName,
			ControllerID:         a.agentTag.Id(),
			StartupValueProvider: startupValueProvider,
			ControllerUUID:       controllerRuntimeConfig.ControllerUUID,
			ControllerModelUUID:  controllerRuntimeConfig.ControllerModelUUID,
			ControllerAgentTag:   a.agentTag,
			ControllerTag:        names.NewControllerTag(controllerRuntimeConfig.ControllerUUID),
			LogDir:               controllerRuntimeConfig.LogDir,
			ConfigChangeSocketPath: path.Join(
				controllerRuntimeConfig.DataDir, "configchange.socket",
			),
			ControlSocketPath: path.Join(
				controllerRuntimeConfig.DataDir, "control.socket",
			),
			DataDir:                           controllerRuntimeConfig.DataDir,
			APIPort:                           controllerRuntimeConfig.APIPort,
			AgentPassword:                     controllerRuntimeConfig.AgentPassword,
			RootDir:                           a.rootDir,
			BootstrapLock:                     a.bootstrapLock,
			ControllerUpgradeLock:             a.controllerUpgradeLock,
			UpgradeDBLock:                     a.upgradeDBLock,
			UpgradeStepsLock:                  a.upgradeStepsLock,
			UpgradeCheckLock:                  a.upgradeCheckLock,
			NewDBWorkerFunc:                   a.newDBWorkerFunc,
			PreUpgradeSteps:                   a.preUpgradeSteps,
			UpgradeSteps:                      a.upgradeSteps,
			LogSink:                           logSink,
			Clock:                             c,
			FlightRecorder:                    flightRecorder,
			ValidateMigration:                 a.validateMigration,
			PrometheusRegisterer:              a.prometheusRegistry,
			UpdateLoggerConfig:                updateAgentConfLogging,
			NewAgentStatusSetter:              a.statusSetter,
			ControllerLeaseDuration:           time.Minute,
			TransactionPruneInterval:          time.Hour,
			RegisterIntrospectionHTTPHandlers: registerIntrospectionHandlers,
			NewModelWorker:                    a.startModelWorkers,
			MuxShutdownWait:                   1 * time.Minute,
			SetupLogging:                      agentconf.SetupAgentLogging,
			DependencyEngineMetrics:           metrics,
			NewEnvironFunc:                    newEnvirons,
		}
		manifolds := agentcontroller.IAASManifolds(manifoldsCfg)
		if controllerRuntimeConfig.LoopbackPreferred {
			manifolds = agentcontroller.CAASManifolds(manifoldsCfg)
		}
		if err := dependency.Install(eng, manifolds); err != nil {
			if err := worker.Stop(eng); err != nil {
				logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
			}
			if err := worker.Stop(flightRecorder); err != nil {
				logger.Errorf(context.TODO(), "while stopping flight recorder with bad"+" manifolds: %v", err)
			}
			return nil, err
		}

		if err := addons.StartIntrospection(addons.IntrospectionConfig{
			AgentDir:           agent.Dir(controllerRuntimeConfig.DataDir, a.agentTag),
			Engine:             eng,
			MachineLock:        nil,
			PrometheusGatherer: a.prometheusRegistry,
			FlightRecorder:     flightRecorder,
			WorkerFunc:         introspection.NewWorker,
			Clock:              c,
			Logger:             logger.Child("introspection"),
		}); err != nil {
			// If the introspection worker failed to start, we just
			// log error but continue. It is very unlikely to happen
			// in the real world as the only issue is connecting to
			// the abstract domain socket and the agent is controlled
			// by the OS to only have one.
			logger.Errorf(context.TODO(), "failed to start introspection worker: %v", err)
		}
		if err := addons.RegisterEngineMetrics(
			a.prometheusRegistry, metrics, eng,
			controllerMetricsSink,
		); err != nil {
			// If the dependency engine metrics fail, continue on.
			// This is unlikely to happen in the real world, but
			// shouldn't stop or bring down an agent.
			logger.Errorf(context.TODO(), "failed to start the dependency engine metrics %v", err)
		}
		return eng, nil
	}
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *ControllerAgent) validateMigration(
	_ context.Context, _ base.APICaller,
) error {
	return nil
}

// statusSetter returns a StatusSetter for use during upgrades.
// The controller agent does not track machine status, so a no-op
// implementation is returned.
func (a *ControllerAgent) statusSetter(
	_ context.Context, _ base.APICaller,
) (upgradesteps.StatusSetter, error) {
	return &noopStatusSetter{}, nil
}

// startModelWorkers starts the set of workers that run for every
// model in each controller, both IAAS and CAAS.
func (a *ControllerAgent) startModelWorkers(
	cfg modelworkermanager.NewModelConfig,
) (worker.Worker, error) {
	controllerRuntimeConfig, err := controllerruntimeconfig.ReadControllerRuntimeConfig(a.controllerRuntimePath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelAgent, err := model.WrapAgent(a, controllerRuntimeConfig.ControllerUUID, cfg.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	config := agentengine.DependencyEngineConfig(
		cfg.ModelMetrics,
		internaldependency.WrapLogger(
			internallogger.GetLogger("juju.worker.dependency"),
		),
	)
	config.IsFatal = model.IsFatal
	config.WorstError = model.WorstError
	config.Filter = model.IgnoreErrRemoved
	engine, err := dependency.NewEngine(config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifoldsCfg := model.ManifoldsConfig{
		Agent:                         modelAgent,
		AgentConfigChanged:            a.configChangedVal,
		Authority:                     cfg.Authority,
		Clock:                         clock.WallClock,
		LoggingContext:                cfg.LoggerContext,
		RunFlagDuration:               time.Minute,
		CharmRevisionUpdateInterval:   24 * time.Hour,
		NewEnvironFunc:                newEnvirons,
		NewContainerBrokerFunc:        newCAASBroker,
		NewMigrationMaster:            migrationmaster.NewWorker,
		OperationPrunerInterval:       24 * time.Hour,
		DomainServices:                cfg.DomainServices,
		ProviderServicesGetter:        cfg.ProviderServicesGetter,
		LeaseManager:                  cfg.LeaseManager,
		HTTPClientGetter:              cfg.HTTPClientGetter,
		APIRemoteRelationClientGetter: cfg.APIRemoteRelationClientGetter,

		ModelUUID:     cfg.ModelUUID,
		AgentTag:      a.agentTag,
		ModelTag:      names.NewModelTag(cfg.ModelUUID),
		DataDir:       controllerRuntimeConfig.DataDir,
		LogDir:        controllerRuntimeConfig.LogDir,
		ControllerTag: names.NewControllerTag(controllerRuntimeConfig.ControllerUUID),
		StartupValueProvider: controllerStartupValueProvider{
			agent:                 a,
			controllerRuntimePath: a.controllerRuntimePath,
		},
		UpdateLoggerConfig: func(loggingConfig string) error {
			return controllerruntimeconfig.ChangeControllerRuntimeConfig(
				a.controllerRuntimePath,
				func(cfg *controllerruntimeconfig.ControllerRuntimeConfig) error {
					cfg.LoggingConfig = loggingConfig
					return nil
				},
			)
		},
	}
	if wrench.IsActive("charmrevision", "shortinterval") {
		interval := 10 * time.Second
		logger.Debugf(context.TODO(), "setting short charmrevision worker interval: %v", interval)
		manifoldsCfg.CharmRevisionUpdateInterval = interval
	}

	applyTestingOverrides(a.CurrentConfig(), &manifoldsCfg)

	var manifolds dependency.Manifolds
	if cfg.ModelType == coremodel.IAAS {
		manifolds = iaasModelManifolds(manifoldsCfg)
	} else {
		manifolds = caasModelManifolds(manifoldsCfg)
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
		}
		return nil, errors.Trace(err)
	}

	return &modelWorker{
		Engine:    engine,
		modelUUID: cfg.ModelUUID,
		metrics:   cfg.ModelMetrics,
	}, nil
}

// logSinkConfigFromRuntimeConfig builds an apiserver.LogSinkConfig from the
// controller runtime config. Zero-value rate-limit fields in the runtime
// config mean "use the default"; non-zero values override the default.
func logSinkConfigFromRuntimeConfig(cfg controllerruntimeconfig.ControllerRuntimeConfig) coreapiserver.LogSinkConfig {
	result := coreapiserver.DefaultLogSinkConfig()
	if cfg.LogSinkRateLimitBurst != 0 {
		result.RateLimitBurst = cfg.LogSinkRateLimitBurst
	}
	if cfg.LogSinkRateLimitRefill != 0 {
		result.RateLimitRefill = cfg.LogSinkRateLimitRefill
	}
	return result
}
