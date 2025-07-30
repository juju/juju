// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"
	"github.com/juju/utils/v4/symlink"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	agentconfig "github.com/juju/juju/agent/config"
	agentengine "github.com/juju/juju/agent/engine"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/caas"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	cmdutil "github.com/juju/juju/cmd/jujud-controller/util"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/container/broker"
	internaldependency "github.com/juju/juju/internal/dependency"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/pki"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage/looputil"
	internalupgrade "github.com/juju/juju/internal/upgrade"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/deployer"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/introspection"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/internal/wrench"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

type (
	// The following allows the upgrade steps to be overridden by brittle
	// integration tests.
	PreUpgradeStepsFunc func(state.ModelType) upgrades.PreUpgradeStepsFunc
	UpgradeStepsFunc    = upgrades.UpgradeStepsFunc
)

var (
	logger            = internallogger.GetLogger("juju.cmd.jujud")
	jujuExec          = paths.JujuExec(paths.CurrentOS())
	jujuDumpLogs      = paths.JujuDumpLogs(paths.CurrentOS())
	jujuIntrospect    = paths.JujuIntrospect(paths.CurrentOS())
	jujudSymlinks     = []string{jujuExec, jujuDumpLogs, jujuIntrospect}
	caasJujudSymlinks = []string{jujuExec, jujuDumpLogs, jujuIntrospect}

	// The following are defined as variables to allow the tests to
	// intercept calls to the functions. In every case, they should
	// be expressed as explicit dependencies, but nobody has yet had
	// the intestinal fortitude to untangle this package. Be that
	// person! Juju Needs You.
	reportOpenedState = func(*state.State) {}
	getHostname       = os.Hostname

	caasModelManifolds   = model.CAASManifolds
	iaasModelManifolds   = model.IAASManifolds
	caasMachineManifolds = machine.CAASManifolds
	iaasMachineManifolds = machine.IAASManifolds
)

type machineAgentFactoryFnType func(names.Tag, bool) (*MachineAgent, error)

// AgentInitializer handles initializing a type for use as a Jujud
// agent.
type AgentInitializer interface {
	AddFlags(*gnuflag.FlagSet)
	CheckArgs([]string) error
	// DataDir returns the directory where this agent should store its data.
	DataDir() string
}

// ModelMetrics defines a type for creating metrics for a given model.
type ModelMetrics interface {
	ForModel(model names.ModelTag) dependency.Metrics
}

// NewMachineAgentCommand creates a Command that handles parsing
// command-line arguments and instantiating and running a
// MachineAgent.
func NewMachineAgentCommand(
	ctx *cmd.Context,
	machineAgentFactory machineAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconfig.AgentConfigWriter,
) cmd.Command {
	return &machineAgentCommand{
		ctx:                 ctx,
		machineAgentFactory: machineAgentFactory,
		agentInitializer:    agentInitializer,
		currentConfig:       configFetcher,
	}
}

type machineAgentCommand struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer    AgentInitializer
	currentConfig       agentconfig.AgentConfigWriter
	machineAgentFactory machineAgentFactoryFnType
	ctx                 *cmd.Context

	// This group is for debugging purposes.
	logToStdErr bool

	isCaas   bool
	agentTag names.Tag

	// The following are set via command-line flags.
	machineId string
	// TODO(controlleragent) - this will be in a new controller agent command
	controllerId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *machineAgentCommand) Init(args []string) error {

	if a.machineId == "" && a.controllerId == "" {
		return errors.New("either machine-id or controller-id must be set")
	}
	if a.machineId != "" && !names.IsValidMachine(a.machineId) {
		return errors.Errorf("--machine-id option must be a non-negative integer")
	}
	if a.controllerId != "" && !names.IsValidControllerAgent(a.controllerId) {
		return errors.Errorf("--controller-id option must be a non-negative integer")
	}
	if err := a.agentInitializer.CheckArgs(args); err != nil {
		return err
	}

	// Due to changes in the logging, and needing to care about old
	// models that have been upgraded, we need to explicitly remove the
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	_, _ = loggo.RemoveWriter("logfile")

	if a.machineId != "" {
		a.agentTag = names.NewMachineTag(a.machineId)
	} else {
		a.agentTag = names.NewControllerAgentTag(a.controllerId)
	}
	if err := agentconfig.ReadAgentConfig(a.currentConfig, a.agentTag.Id()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}
	config := a.currentConfig.CurrentConfig()
	if err := os.MkdirAll(config.LogDir(), 0644); err != nil {
		logger.Warningf(context.TODO(), "cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	if !a.logToStdErr {
		// the context's stderr is set as the loggo writer in github.com/juju/juju/internal/cmd/logging.go
		ljLogger := &lumberjack.Logger{
			Filename:   agent.LogFilename(config), // eg: "/var/log/juju/machine-0.log"
			MaxSize:    config.AgentLogfileMaxSizeMB(),
			MaxBackups: config.AgentLogfileMaxBackups(),
			Compress:   true,
		}
		logger.Debugf(context.TODO(), "created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		a.ctx.Stderr = ljLogger
	}

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *machineAgentCommand) Run(c *cmd.Context) error {
	machineAgent, err := a.machineAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return machineAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *machineAgentCommand) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.machineId, "machine-id", "", "id of the machine to run")
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
	f.BoolVar(&a.logToStdErr, "log-to-stderr", false, "log to stderr instead of logsink.log")
}

// Info returns usage information for the command.
func (a *machineAgentCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "machine",
		Purpose: "run a juju machine agent",
	})
}

// MachineAgentFactoryFn returns a function which instantiates a
// MachineAgent given a machineId.
func MachineAgentFactoryFn(
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	preUpgradeSteps PreUpgradeStepsFunc,
	upgradeSteps UpgradeStepsFunc,
	rootDir string,
) machineAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*MachineAgent, error) {
		runner, err := worker.NewRunner(worker.RunnerParams{
			Name:          "machine",
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  internalworker.RestartDelay,
			Logger:        internalworker.WrapLogger(logger),
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return NewMachineAgent(
			agentTag,
			agentConfWriter,
			runner,
			looputil.NewLoopDeviceManager(),
			newDBWorkerFunc,
			preUpgradeSteps,
			upgradeSteps,
			rootDir,
			isCaasAgent,
		)
	}
}

// NewMachineAgent instantiates a new MachineAgent.
func NewMachineAgent(
	agentTag names.Tag,
	agentConfWriter agentconfig.AgentConfigWriter,
	runner *worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	preUpgradeSteps PreUpgradeStepsFunc,
	upgradeSteps UpgradeStepsFunc,
	rootDir string,
	isCaasAgent bool,
) (*MachineAgent, error) {
	prometheusRegistry, err := addons.NewPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	a := &MachineAgent{
		agentTag:                    agentTag,
		AgentConfigWriter:           agentConfWriter,
		configChangedVal:            voyeur.NewValue(true),
		workersStarted:              make(chan struct{}),
		dead:                        make(chan struct{}),
		runner:                      runner,
		rootDir:                     rootDir,
		initialUpgradeCheckComplete: gate.NewLock(),
		newDBWorkerFunc:             newDBWorkerFunc,
		loopDeviceManager:           loopDeviceManager,
		prometheusRegistry:          prometheusRegistry,
		preUpgradeSteps:             preUpgradeSteps,
		upgradeSteps:                upgradeSteps,
		isCaasAgent:                 isCaasAgent,
		cmdRunner:                   &defaultRunner{},
	}
	return a, nil
}

func (a *MachineAgent) registerPrometheusCollectors() error {
	return nil
}

// CommandRunner allows to run commands on the underlying system
type CommandRunner interface {
	RunCommands(run exec.RunParams) (*exec.ExecResponse, error)
}

type defaultRunner struct{}

// RunCommands executes the Commands specified in the RunParams using
// '/bin/bash -s' on everything else, passing the commands through as
// stdin, and collecting stdout and stderr. If a non-zero return code is
// returned, this is collected as the code for the response and this does
// not classify as an error.
func (defaultRunner) RunCommands(run exec.RunParams) (*exec.ExecResponse, error) {
	return exec.RunCommands(run)
}

// MachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type MachineAgent struct {
	agentconfig.AgentConfigWriter

	ctx              *cmd.Context
	dead             chan struct{}
	errReason        error
	agentTag         names.Tag
	runner           *worker.Runner
	rootDir          string
	configChangedVal *voyeur.Value

	workersStarted chan struct{}
	machineLock    machinelock.Lock

	newDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	initialUpgradeCheckComplete gate.Lock

	mongoInitMutex   sync.Mutex
	mongoInitialized bool

	loopDeviceManager  looputil.LoopDeviceManager
	prometheusRegistry *prometheus.Registry

	// To allow for testing in legacy tests (brittle integration tests), we
	// need to override these.
	preUpgradeSteps PreUpgradeStepsFunc
	upgradeSteps    UpgradeStepsFunc

	bootstrapLock    gate.Lock
	upgradeDBLock    gate.Lock
	upgradeStepsLock gate.Lock

	isCaasAgent bool
	cmdRunner   CommandRunner
}

// Wait waits for the machine agent to finish.
func (a *MachineAgent) Wait() error {
	<-a.dead
	return a.errReason
}

// Stop stops the machine agent.
func (a *MachineAgent) Stop() error {
	a.runner.Kill()
	return a.Wait()
}

// Done signals the machine agent is finished
func (a *MachineAgent) Done(err error) {
	a.errReason = err
	close(a.dead)
}

// upgradeCertificateDNSNames ensure that the controller certificate
// recorded in the agent config and also mongo server.pem contains the
// DNSNames entries required by Juju.
func upgradeCertificateDNSNames(config agent.ConfigSetter) error {
	si, ok := config.ControllerAgentInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return nil
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey([]byte(config.CACert()),
		[]byte(si.CAPrivateKey))
	if err != nil {
		return errors.Annotate(err, "building authority from ca pem")
	}

	// Validate the current certificate and private key pair, and then
	// extract the current DNS names from the certificate. If the
	// certificate validation fails, or it does not contain the DNS
	// names we require, we will generate a new one.
	leaf, err := authority.LeafGroupFromPemCertKey(pki.DefaultLeafGroup,
		[]byte(si.Cert), []byte(si.PrivateKey))
	if err != nil || !pki.LeafHasDNSNames(leaf, controller.DefaultDNSNames) {
		logger.Infof(context.TODO(), "parsing certificate/key failed, will generate a new one: %v", err)
		leaf, err = authority.LeafRequestForGroup(pki.DefaultLeafGroup).
			AddDNSNames(controller.DefaultDNSNames...).
			Commit()
		if err != nil {
			return errors.Annotate(err, "generating new default controller certificate")
		}
	}

	cert, privateKey, err := leaf.ToPemParts()
	if err != nil {
		return errors.Annotate(err, "transforming controller certificate to pem format")
	}

	si.Cert, si.PrivateKey = string(cert), string(privateKey)

	config.SetControllerAgentInfo(si)
	return nil
}

// Run runs a machine agent.
func (a *MachineAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)
	a.ctx = ctx
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentconf.SetupAgentLogging(internallogger.DefaultContext(), a.CurrentConfig())

	// Prime the log sink and create the writer.
	logSink, err := PrimeLogSink(a.CurrentConfig())
	if err != nil {
		return errors.Trace(err)
	}
	defer logSink.Close()

	// Add the log sink to the default logger context.
	if err := loggo.DefaultContext().AddWriter("logsink", corelogger.NewTaggedRedirectWriter(
		logSink,
		a.Tag().String(),
		a.CurrentConfig().Model().Id(),
	)); err != nil {
		return errors.Trace(err)
	}

	if err := introspection.WriteProfileFunctions(introspection.ProfileDir); err != nil {
		// This isn't fatal, just annoying.
		logger.Errorf(context.Background(), "failed to write profile funcs: %v", err)
	}

	// Before doing anything else, we need to make sure the certificate
	// generated for use by mongo to validate controller connections is correct.
	// This needs to be done before any possible restart of the mongo service.
	// See bug http://pad.lv/1434680
	if err := a.AgentConfigWriter.ChangeConfig(upgradeCertificateDNSNames); err != nil {
		return errors.Annotate(err, "error upgrading server certificate")
	}

	// Moved from NewMachineAgent here because the agent config could not be
	// ready yet there.
	if err := a.registerPrometheusCollectors(); err != nil {
		return errors.Trace(err)
	}

	agentConfig := a.CurrentConfig()
	agentName := a.Tag().String()
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   agentName,
		Clock:       clock.WallClock,
		Logger:      internallogger.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(agentConfig),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		return errors.Trace(err)
	}
	a.machineLock = machineLock

	a.bootstrapLock = gate.NewLock()
	a.upgradeDBLock = internalupgrade.NewLock(agentConfig, jujuversion.Current)
	a.upgradeStepsLock = internalupgrade.NewLock(agentConfig, jujuversion.Current)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion(), logSink)
	if err := a.createJujudSymlinks(agentConfig.DataDir()); err != nil {
		return err
	}
	_ = a.runner.StartWorker(ctx, "engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err = a.runner.Wait()
	switch errors.Cause(err) {
	case internalworker.ErrRebootMachine:
		logger.Infof(ctx, "Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case internalworker.ErrShutdownMachine:
		logger.Infof(ctx, "Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	return cmdutil.AgentDone(logger, err)
}

func (a *MachineAgent) makeEngineCreator(
	agentName string, previousAgentVersion semversion.Number,
	logSink corelogger.LogSink,
) func(context.Context) (worker.Worker, error) {
	return func(ctx context.Context) (worker.Worker, error) {
		agentConfig := a.CurrentConfig()
		engineConfigFunc := agentengine.DependencyEngineConfig
		metrics := agentengine.NewMetrics()
		controllerMetricsSink := metrics.ForModel(agentConfig.Model())
		eng, err := dependency.NewEngine(engineConfigFunc(
			controllerMetricsSink,
			internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
		))
		if err != nil {
			return nil, err
		}
		updateAgentConfLogging := func(loggingConfig string) error {
			return a.AgentConfigWriter.ChangeConfig(func(setter agent.ConfigSetter) error {
				setter.SetLoggingConfig(loggingConfig)
				return nil
			})
		}

		registerIntrospectionHandlers := func(handle func(path string, h http.Handler)) {
			handle("/metrics/", promhttp.HandlerFor(a.prometheusRegistry, promhttp.HandlerOpts{}))
		}

		manifoldsCfg := machine.ManifoldsConfig{
			PreviousAgentVersion:              previousAgentVersion,
			AgentName:                         agentName,
			Agent:                             agent.APIHostPortsSetter{Agent: a},
			RootDir:                           a.rootDir,
			AgentConfigChanged:                a.configChangedVal,
			BootstrapLock:                     a.bootstrapLock,
			UpgradeDBLock:                     a.upgradeDBLock,
			UpgradeStepsLock:                  a.upgradeStepsLock,
			UpgradeCheckLock:                  a.initialUpgradeCheckComplete,
			NewDBWorkerFunc:                   a.newDBWorkerFunc,
			PreUpgradeSteps:                   a.preUpgradeSteps,
			UpgradeSteps:                      a.upgradeSteps,
			LogSink:                           logSink,
			NewDeployContext:                  deployer.NewNestedContext,
			Clock:                             clock.WallClock,
			ValidateMigration:                 a.validateMigration,
			PrometheusRegisterer:              a.prometheusRegistry,
			UpdateLoggerConfig:                updateAgentConfLogging,
			NewAgentStatusSetter:              a.statusSetter,
			ControllerLeaseDuration:           time.Minute,
			TransactionPruneInterval:          time.Hour,
			MachineLock:                       a.machineLock,
			RegisterIntrospectionHTTPHandlers: registerIntrospectionHandlers,
			NewModelWorker:                    a.startModelWorkers,
			MuxShutdownWait:                   1 * time.Minute,
			NewBrokerFunc:                     newBroker,
			MachineStartup:                    a.machineStartup,
			IsCaasConfig:                      a.isCaasAgent,
			UnitEngineConfig: func() dependency.EngineConfig {
				return agentengine.DependencyEngineConfig(
					controllerMetricsSink,
					internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
				)
			},
			SetupLogging:            agentconf.SetupAgentLogging,
			DependencyEngineMetrics: metrics,
			NewEnvironFunc:          newEnvirons,
			NewCAASBrokerFunc:       newCAASBroker,
		}
		manifolds := iaasMachineManifolds(manifoldsCfg)
		if a.isCaasAgent {
			manifolds = caasMachineManifolds(manifoldsCfg)
		}
		if err := dependency.Install(eng, manifolds); err != nil {
			if err := worker.Stop(eng); err != nil {
				logger.Errorf(context.TODO(), "while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		if err := addons.StartIntrospection(addons.IntrospectionConfig{
			AgentDir:           agentConfig.Dir(),
			Engine:             eng,
			MachineLock:        a.machineLock,
			PrometheusGatherer: a.prometheusRegistry,
			WorkerFunc:         introspection.NewWorker,
			Clock:              clock.WallClock,
			Logger:             logger.Child("introspection"),
		}); err != nil {
			// If the introspection worker failed to start, we just log error
			// but continue. It is very unlikely to happen in the real world
			// as the only issue is connecting to the abstract domain socket
			// and the agent is controlled by by the OS to only have one.
			logger.Errorf(context.TODO(), "failed to start introspection worker: %v", err)
		}
		if err := addons.RegisterEngineMetrics(a.prometheusRegistry, metrics, eng, controllerMetricsSink); err != nil {
			// If the dependency engine metrics fail, continue on. This is
			// unlikely to happen in the real world, but shouldn't stop or
			// bring down an agent.
			logger.Errorf(context.TODO(), "failed to start the dependency engine metrics %v", err)
		}
		return eng, nil
	}
}

func (a *MachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(a.CurrentConfig())
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof(context.TODO(), "Reboot: Executing reboot")
	err = finalize.ExecuteReboot(action)
	if err != nil {
		logger.Infof(context.TODO(), "Reboot: Error executing reboot: %v", err)
		return errors.Trace(err)
	}
	// We return ErrRebootMachine so the agent will simply exit without error
	// pending reboot/shutdown.
	return internalworker.ErrRebootMachine
}

func (a *MachineAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

var (
	newEnvirons   = environs.New
	newCAASBroker = caas.New
	newBroker     = broker.New
)

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter
func (a *noopStatusSetter) SetStatus(_ context.Context, _ status.Status, _ string, _ map[string]interface{}) error {
	return nil
}

func (a *MachineAgent) statusSetter(ctx context.Context, apiCaller base.APICaller) (upgradesteps.StatusSetter, error) {
	if a.isCaasAgent || a.agentTag.Kind() != names.MachineTagKind {
		// TODO - support set status for controller agents
		return &noopStatusSetter{}, nil
	}
	machinerAPI := apimachiner.NewClient(apiCaller)
	return machinerAPI.Machine(ctx, a.Tag().(names.MachineTag))
}

// Restart restarts the agent's service.
func (a *MachineAgent) Restart() error {
	// TODO(bootstrap): revisit here to make it only invoked by IAAS.
	name := a.CurrentConfig().Value(agent.AgentServiceName)
	return service.Restart(name)
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *MachineAgent) validateMigration(ctx context.Context, apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	var err error
	// TODO(controlleragent) - add k8s controller check.
	if !a.isCaasAgent {
		facade := apimachiner.NewClient(apiCaller)
		_, err = facade.Machine(ctx, a.agentTag.(names.MachineTag))
	}
	return errors.Trace(err)
}

// startModelWorkers starts the set of workers that run for every model
// in each controller, both IAAS and CAAS.
func (a *MachineAgent) startModelWorkers(cfg modelworkermanager.NewModelConfig) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()
	controllerUUID := currentConfig.Controller().Id()
	modelAgent, err := model.WrapAgent(a, controllerUUID, cfg.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	config := agentengine.DependencyEngineConfig(
		cfg.ModelMetrics,
		internaldependency.WrapLogger(internallogger.GetLogger("juju.worker.dependency")),
	)
	config.IsFatal = model.IsFatal
	config.WorstError = model.WorstError
	config.Filter = model.IgnoreErrRemoved
	engine, err := dependency.NewEngine(config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifoldsCfg := model.ManifoldsConfig{
		Agent:                       modelAgent,
		AgentConfigChanged:          a.configChangedVal,
		Authority:                   cfg.Authority,
		Clock:                       clock.WallClock,
		LoggingContext:              cfg.LoggerContext,
		RunFlagDuration:             time.Minute,
		CharmRevisionUpdateInterval: 24 * time.Hour,
		NewEnvironFunc:              newEnvirons,
		NewContainerBrokerFunc:      newCAASBroker,
		NewMigrationMaster:          migrationmaster.NewWorker,
		DomainServices:              cfg.DomainServices,
		ProviderServicesGetter:      cfg.ProviderServicesGetter,
		LeaseManager:                cfg.LeaseManager,
		HTTPClientGetter:            cfg.HTTPClientGetter,
	}
	if wrench.IsActive("charmrevision", "shortinterval") {
		interval := 10 * time.Second
		logger.Debugf(context.TODO(), "setting short charmrevision worker interval: %v", interval)
		manifoldsCfg.CharmRevisionUpdateInterval = interval
	}

	applyTestingOverrides(currentConfig, &manifoldsCfg)

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

func applyTestingOverrides(agentConfig agent.Config, manifoldsCfg *model.ManifoldsConfig) {
	if v := agentConfig.Value(agent.CharmRevisionUpdateInterval); v != "" {
		charmRevisionUpdateInterval, err := time.ParseDuration(v)
		if err == nil {
			manifoldsCfg.CharmRevisionUpdateInterval = charmRevisionUpdateInterval
			logger.Infof(context.TODO(), "model worker charm revision update interval set to %v for testing",
				charmRevisionUpdateInterval)
		} else {
			logger.Warningf(context.TODO(), "invalid charm revision update interval, using default %v: %v",
				manifoldsCfg.CharmRevisionUpdateInterval, err)
		}
	}
}

type modelWorker struct {
	*dependency.Engine
	modelUUID string
	metrics   agentengine.MetricSink
}

// Wait is the last thing that is called on the worker as it is being
// removed.
func (m *modelWorker) Wait() error {
	err := m.Engine.Wait()

	// When closing the model, ensure that we also close the metrics with the
	// logger.
	_ = m.metrics.Unregister()
	return err
}

func openStatePool(
	agentConfig agent.Config,
	domainServicesGetter services.DomainServicesGetter,
) (_ *state.StatePool, err error) {
	storageServiceGetter := func(modelUUID coremodel.UUID) (state.StoragePoolGetter, error) {
		svc, err := domainServicesGetter.ServicesForModel(context.Background(), modelUUID)
		if err != nil {
			return nil, err
		}
		return svc.Storage(), nil
	}

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      agentConfig.Controller(),
		ControllerModelTag: agentConfig.Model(),
		NewPolicy:          stateenvirons.GetNewPolicyFunc(storageServiceGetter),
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// WorkersStarted returns a channel that's closed once all top level workers
// have been started. This is provided for testing purposes.
func (a *MachineAgent) WorkersStarted() <-chan struct{} {
	return a.workersStarted
}

func (a *MachineAgent) Tag() names.Tag {
	return a.agentTag
}

func (a *MachineAgent) createJujudSymlinks(dataDir string) error {
	jujud := filepath.Join(tools.ToolsDir(dataDir, a.Tag().String()), jujunames.Jujud)
	symlinks := jujudSymlinks
	if a.isCaasAgent {
		// For IAAS, this is done in systemd for for caas we need to do it here.
		caasJujud := filepath.Join(tools.ToolsDir(dataDir, ""), jujunames.Jujud)
		if err := a.createSymlink(caasJujud, jujud); err != nil {
			return errors.Annotatef(err, "failed to create %s symlink", jujud)
		}
		symlinks = caasJujudSymlinks
	}
	for _, link := range symlinks {
		err := a.createSymlink(jujud, link)
		if err != nil {
			return errors.Annotatef(err, "failed to create %s symlink", link)
		}
	}
	return nil
}

func (a *MachineAgent) createSymlink(target, link string) error {
	fullLink := utils.EnsureBaseDir(a.rootDir, link)

	// TODO(juju 4) - remove this legacy behaviour.
	// Remove the obsolete "juju-run" symlink
	if strings.Contains(fullLink, "/juju-exec") {
		runLink := strings.Replace(fullLink, "/juju-exec", "/juju-run", 1)
		_ = os.Remove(runLink)
	}

	if stat, err := os.Lstat(fullLink); err == nil {
		if stat.Mode()&os.ModeSymlink == 0 {
			logger.Infof(context.TODO(), "skipping creating symlink %q as exsting path has a normal file", fullLink)
			return nil
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Annotatef(err, "cannot check if %q is a symlink", fullLink)
	}

	currentTarget, err := symlink.Read(fullLink)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err == nil {
		// Link already in place - check it.
		if currentTarget == target {
			// Link already points to the right place - nothing to do.
			return nil
		}
		// Link points to the wrong place - delete it.
		if err := os.Remove(fullLink); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(fullLink), os.FileMode(0755)); err != nil {
		return err
	}
	return symlink.New(target, fullLink)
}

func (a *MachineAgent) machineStartup(ctx context.Context, apiConn api.Connection, logger corelogger.Logger) error {
	logger.Tracef(ctx, "machineStartup called")
	// CAAS agents do not have machines.
	if a.isCaasAgent {
		return nil
	}

	// Report the machine host name and record the agent start time. This
	// ensures that whenever a machine restarts, the instancepoller gets a
	// chance to immediately refresh the provider address (inc. shadow IP)
	// information which can change between reboots.
	hostname, err := getHostname()
	if err != nil {
		return errors.Annotate(err, "getting machine hostname")
	}
	if err := a.recordAgentStartInformation(ctx, apiConn, hostname); err != nil {
		return errors.Annotate(err, "recording agent start information")
	}

	return nil
}

func (a *MachineAgent) machine(ctx context.Context, apiConn api.Connection) (*apimachiner.Machine, error) {
	machinerAPI := apimachiner.NewClient(apiConn)
	agentConfig := a.CurrentConfig()

	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("%q is not a machine tag", agentConfig.Tag().String())
	}
	return machinerAPI.Machine(ctx, tag)
}

func (a *MachineAgent) recordAgentStartInformation(ctx context.Context, apiConn api.Connection, hostname string) error {
	m, err := a.machine(ctx, apiConn)
	if errors.Is(err, errors.NotFound) || err == nil && m.Life() == life.Dead {
		return internalworker.ErrTerminateAgent
	}
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", a.CurrentConfig().Tag())
	}

	if err := m.RecordAgentStartInformation(ctx, hostname); err != nil {
		return errors.Annotate(err, "cannot record agent start information")
	}
	return nil
}

// statePoolIntrospectionReporter wraps a (possibly nil) state.StatePool,
// calling its IntrospectionReport method or returning a message if it
// is nil.
type statePoolIntrospectionReporter struct {
	mu   sync.Mutex
	pool *state.StatePool
}

func (h *statePoolIntrospectionReporter) Set(pool *state.StatePool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pool = pool
}

func (h *statePoolIntrospectionReporter) IntrospectionReport() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pool == nil {
		return "agent has no pool set"
	}
	return h.pool.IntrospectionReport()
}
