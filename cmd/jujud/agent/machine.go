// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/lumberjack"
	"github.com/juju/mgo/v2"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub/v2"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/symlink"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	apimachiner "github.com/juju/juju/api/machiner"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/cmd/jujud/reboot"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/broker"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongometrics"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/service"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/upgrades"
	jworker "github.com/juju/juju/worker"
	workercommon "github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/conv2state"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/logsender/logsendermetrics"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/modelworkermanager"
	"github.com/juju/juju/worker/provisioner"
	psworker "github.com/juju/juju/worker/pubsub"
	"github.com/juju/juju/worker/upgradedatabase"
	"github.com/juju/juju/worker/upgradesteps"
	"github.com/juju/juju/wrench"
)

var (
	logger            = loggo.GetLogger("juju.cmd.jujud")
	jujuRun           = paths.JujuRun(paths.CurrentOS())
	jujuDumpLogs      = paths.JujuDumpLogs(paths.CurrentOS())
	jujuIntrospect    = paths.JujuIntrospect(paths.CurrentOS())
	jujudSymlinks     = []string{jujuRun, jujuDumpLogs, jujuIntrospect}
	caasJujudSymlinks = []string{jujuRun, jujuDumpLogs, jujuIntrospect}

	// The following are defined as variables to allow the tests to
	// intercept calls to the functions. In every case, they should
	// be expressed as explicit dependencies, but nobody has yet had
	// the intestinal fortitude to untangle this package. Be that
	// person! Juju Needs You.
	useMultipleCPUs   = utils.UseMultipleCPUs
	reportOpenedState = func(*state.State) {}
	getHostname       = os.Hostname

	caasModelManifolds   = model.CAASManifolds
	iaasModelManifolds   = model.IAASManifolds
	caasMachineManifolds = machine.CAASManifolds
	iaasMachineManifolds = machine.IAASManifolds
)

// Variable to override in tests, default is true
var ProductionMongoWriteConcern = true

func init() {
	stateWorkerDialOpts = mongo.DefaultDialOpts()
	stateWorkerDialOpts.PostDial = func(session *mgo.Session) error {
		safe := mgo.Safe{}
		if ProductionMongoWriteConcern {
			safe.J = true
			_, err := mongo.CurrentReplicasetConfig(session)
			if err == nil {
				// set mongo to write-majority (writes only returned after
				// replicated to a majority of replica-set members).
				safe.WMode = "majority"
			}
		}
		session.SetSafe(&safe)
		return nil
	}
}

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

// NewMachineAgentCmd creates a Command which handles parsing
// command-line arguments and instantiating and running a
// MachineAgent.
func NewMachineAgentCmd(
	ctx *cmd.Context,
	machineAgentFactory machineAgentFactoryFnType,
	agentInitializer AgentInitializer,
	configFetcher agentconf.AgentConfigWriter,
) cmd.Command {
	return &machineAgentCmd{
		ctx:                 ctx,
		machineAgentFactory: machineAgentFactory,
		agentInitializer:    agentInitializer,
		currentConfig:       configFetcher,
	}
}

type machineAgentCmd struct {
	cmd.CommandBase

	// This group of arguments is required.
	agentInitializer    AgentInitializer
	currentConfig       agentconf.AgentConfigWriter
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
func (a *machineAgentCmd) Init(args []string) error {

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
	if err := agentconf.ReadAgentConfig(a.currentConfig, a.agentTag.Id()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}
	config := a.currentConfig.CurrentConfig()
	if err := os.MkdirAll(config.LogDir(), 0644); err != nil {
		logger.Warningf("cannot create log dir: %v", err)
	}
	a.isCaas = config.Value(agent.ProviderType) == k8sconstants.CAASProviderType

	if !a.logToStdErr {
		// the context's stderr is set as the loggo writer in github.com/juju/cmd/v3/logging.go
		ljLogger := &lumberjack.Logger{
			Filename:   agent.LogFilename(config), // eg: "/var/log/juju/machine-0.log"
			MaxSize:    config.AgentLogfileMaxSizeMB(),
			MaxBackups: config.AgentLogfileMaxBackups(),
			Compress:   true,
		}
		logger.Debugf("created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		a.ctx.Stderr = ljLogger
	}

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *machineAgentCmd) Run(c *cmd.Context) error {
	machineAgent, err := a.machineAgentFactory(a.agentTag, a.isCaas)
	if err != nil {
		return errors.Trace(err)
	}
	return machineAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *machineAgentCmd) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.machineId, "machine-id", "", "id of the machine to run")
	f.StringVar(&a.controllerId, "controller-id", "", "id of the controller to run")
	f.BoolVar(&a.logToStdErr, "log-to-stderr", false, "log to stderr instead of logsink.log")
}

// Info returns usage information for the command.
func (a *machineAgentCmd) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "machine",
		Purpose: "run a juju machine agent",
	})
}

// MachineAgentFactoryFn returns a function which instantiates a
// MachineAgent given a machineId.
func MachineAgentFactoryFn(
	agentConfWriter agentconf.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	newIntrospectionSocketName func(names.Tag) string,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	rootDir string,
) machineAgentFactoryFnType {
	return func(agentTag names.Tag, isCaasAgent bool) (*MachineAgent, error) {
		return NewMachineAgent(
			agentTag,
			agentConfWriter,
			bufferedLogger,
			worker.NewRunner(worker.RunnerParams{
				IsFatal:       agenterrors.IsFatal,
				MoreImportant: agenterrors.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
			}),
			looputil.NewLoopDeviceManager(),
			newIntrospectionSocketName,
			preUpgradeSteps,
			rootDir,
			isCaasAgent,
		)
	}
}

// NewMachineAgent instantiates a new MachineAgent.
func NewMachineAgent(
	agentTag names.Tag,
	agentConfWriter agentconf.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	runner *worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
	newIntrospectionSocketName func(names.Tag) string,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
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
		bufferedLogger:              bufferedLogger,
		workersStarted:              make(chan struct{}),
		dead:                        make(chan struct{}),
		runner:                      runner,
		rootDir:                     rootDir,
		initialUpgradeCheckComplete: gate.NewLock(),
		loopDeviceManager:           loopDeviceManager,
		newIntrospectionSocketName:  newIntrospectionSocketName,
		prometheusRegistry:          prometheusRegistry,
		mongoTxnCollector:           mongometrics.NewTxnCollector(),
		mongoDialCollector:          mongometrics.NewDialCollector(),
		preUpgradeSteps:             preUpgradeSteps,
		isCaasAgent:                 isCaasAgent,
	}
	return a, nil
}

func (a *MachineAgent) registerPrometheusCollectors() error {
	agentConfig := a.CurrentConfig()
	if v := agentConfig.Value(agent.MgoStatsEnabled); v == "true" {
		// Enable mgo stats collection only if requested,
		// as it may affect performance.
		mgo.SetStats(true)
		collector := mongometrics.NewMgoStatsCollector(mgo.GetStats)
		if err := a.prometheusRegistry.Register(collector); err != nil {
			return errors.Annotate(err, "registering mgo stats collector")
		}
	}
	if err := a.prometheusRegistry.Register(
		logsendermetrics.BufferedLogWriterMetrics{BufferedLogWriter: a.bufferedLogger},
	); err != nil {
		return errors.Annotate(err, "registering logsender collector")
	}
	if err := a.prometheusRegistry.Register(a.mongoTxnCollector); err != nil {
		return errors.Annotate(err, "registering mgo/txn collector")
	}
	if err := a.prometheusRegistry.Register(a.mongoDialCollector); err != nil {
		return errors.Annotate(err, "registering mongo dial collector")
	}
	if err := a.prometheusRegistry.Register(a.pubsubMetrics); err != nil {
		return errors.Annotate(err, "registering pubsub collector")
	}
	return nil
}

// MachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type MachineAgent struct {
	agentconf.AgentConfigWriter

	ctx               *cmd.Context
	dead              chan struct{}
	errReason         error
	agentTag          names.Tag
	runner            *worker.Runner
	rootDir           string
	bufferedLogger    *logsender.BufferedLogWriter
	configChangedVal  *voyeur.Value
	dbUpgradeComplete gate.Lock
	upgradeComplete   gate.Lock
	workersStarted    chan struct{}
	machineLock       machinelock.Lock

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	initialUpgradeCheckComplete gate.Lock

	mongoInitMutex   sync.Mutex
	mongoInitialized bool

	loopDeviceManager          looputil.LoopDeviceManager
	newIntrospectionSocketName func(names.Tag) string
	prometheusRegistry         *prometheus.Registry
	mongoTxnCollector          *mongometrics.TxnCollector
	mongoDialCollector         *mongometrics.DialCollector
	preUpgradeSteps            upgrades.PreUpgradeStepsFunc

	// Only API servers have hubs. This is temporary until the apiserver and
	// peergrouper have manifolds.
	centralHub    *pubsub.StructuredHub
	pubsubMetrics *centralhub.PubsubMetrics

	isCaasAgent bool
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
	si, ok := config.StateServingInfo()
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
		logger.Infof("parsing certificate/key failed, will generate a new one: %v", err)
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

	if err := mongo.UpdateSSLKey(config.DataDir(), si.Cert, si.PrivateKey); err != nil {
		return err
	}
	config.SetStateServingInfo(si)
	return nil
}

// Run runs a machine agent.
func (a *MachineAgent) Run(ctx *cmd.Context) (err error) {
	defer a.Done(err)
	a.ctx = ctx
	useMultipleCPUs()
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	agentconf.SetupAgentLogging(loggo.DefaultContext(), a.CurrentConfig())

	if err := introspection.WriteProfileFunctions(introspection.ProfileDir); err != nil {
		// This isn't fatal, just annoying.
		logger.Errorf("failed to write profile funcs: %v", err)
	}

	// When the API server and peergrouper have manifolds, they can
	// have dependencies on a central hub worker.
	a.pubsubMetrics = centralhub.NewPubsubMetrics()
	a.centralHub = centralhub.New(a.Tag(), a.pubsubMetrics)

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
		Logger:      loggo.GetLogger("juju.machinelock"),
		LogFilename: agent.MachineLockLogFilename(agentConfig),
	})
	// There will only be an error if the required configuration
	// values are not passed in.
	if err != nil {
		return errors.Trace(err)
	}
	a.machineLock = machineLock
	a.dbUpgradeComplete = upgradedatabase.NewLock(agentConfig)
	a.upgradeComplete = upgradesteps.NewLock(agentConfig)

	createEngine := a.makeEngineCreator(agentName, agentConfig.UpgradedToVersion())
	if err := a.createJujudSymlinks(agentConfig.DataDir()); err != nil {
		return err
	}
	_ = a.runner.StartWorker("engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err = a.runner.Wait()
	switch errors.Cause(err) {
	case jworker.ErrRebootMachine:
		logger.Infof("Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case jworker.ErrShutdownMachine:
		logger.Infof("Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	return cmdutil.AgentDone(logger, err)
}

func (a *MachineAgent) makeEngineCreator(
	agentName string, previousAgentVersion version.Number,
) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		engineConfigFunc := engine.DependencyEngineConfig
		metrics := engine.NewMetrics()
		controllerMetricsSink := metrics.ForModel(a.CurrentConfig().Model())
		engine, err := dependency.NewEngine(engineConfigFunc(controllerMetricsSink))
		if err != nil {
			return nil, err
		}
		localHub := pubsub.NewSimpleHub(&pubsub.SimpleHubConfig{
			Logger: loggo.GetLogger("juju.localhub"),
		})
		pubsubReporter := psworker.NewReporter()
		presenceRecorder := presence.New(clock.WallClock)
		updateAgentConfLogging := func(loggingConfig string) error {
			return a.AgentConfigWriter.ChangeConfig(func(setter agent.ConfigSetter) error {
				setter.SetLoggingConfig(loggingConfig)
				return nil
			})
		}
		updateControllerAPIPort := func(port int) error {
			return a.AgentConfigWriter.ChangeConfig(func(setter agent.ConfigSetter) error {
				setter.SetControllerAPIPort(port)
				return nil
			})
		}

		// statePoolReporter is an introspection.IntrospectionReporter,
		// which is set to the current StatePool managed by the state
		// tracker in controller agents.
		var statePoolReporter statePoolIntrospectionReporter
		registerIntrospectionHandlers := func(handle func(path string, h http.Handler)) {
			handle("/metrics/", promhttp.HandlerFor(a.prometheusRegistry, promhttp.HandlerOpts{}))
		}

		manifoldsCfg := machine.ManifoldsConfig{
			PreviousAgentVersion:    previousAgentVersion,
			AgentName:               agentName,
			Agent:                   agent.APIHostPortsSetter{Agent: a},
			RootDir:                 a.rootDir,
			AgentConfigChanged:      a.configChangedVal,
			UpgradeDBLock:           a.dbUpgradeComplete,
			UpgradeStepsLock:        a.upgradeComplete,
			UpgradeCheckLock:        a.initialUpgradeCheckComplete,
			OpenController:          a.initController,
			OpenStatePool:           a.initState,
			OpenStateForUpgrade:     a.openStateForUpgrade,
			StartAPIWorkers:         a.startAPIWorkers,
			PreUpgradeSteps:         a.preUpgradeSteps,
			LogSource:               a.bufferedLogger.Logs(),
			NewDeployContext:        deployer.NewNestedContext,
			Clock:                   clock.WallClock,
			ValidateMigration:       a.validateMigration,
			PrometheusRegisterer:    a.prometheusRegistry,
			CentralHub:              a.centralHub,
			LocalHub:                localHub,
			PubSubReporter:          pubsubReporter,
			PresenceRecorder:        presenceRecorder,
			UpdateLoggerConfig:      updateAgentConfLogging,
			UpdateControllerAPIPort: updateControllerAPIPort,
			NewAgentStatusSetter: func(apiConn api.Connection) (upgradesteps.StatusSetter, error) {
				return a.statusSetter(apiConn)
			},
			ControllerLeaseDuration:           time.Minute,
			LogPruneInterval:                  5 * time.Minute,
			TransactionPruneInterval:          time.Hour,
			MachineLock:                       a.machineLock,
			SetStatePool:                      statePoolReporter.Set,
			RegisterIntrospectionHTTPHandlers: registerIntrospectionHandlers,
			NewModelWorker:                    a.startModelWorkers,
			MuxShutdownWait:                   1 * time.Minute,
			NewContainerBrokerFunc:            newCAASBroker,
			NewBrokerFunc:                     newBroker,
			IsCaasConfig:                      a.isCaasAgent,
			UnitEngineConfig: func() dependency.EngineConfig {
				return engineConfigFunc(controllerMetricsSink)
			},
			SetupLogging:            agentconf.SetupAgentLogging,
			LeaseFSM:                raftlease.NewFSM(),
			RaftOpQueue:             queue.NewOpQueue(clock.WallClock),
			DependencyEngineMetrics: metrics,
		}
		manifolds := iaasMachineManifolds(manifoldsCfg)
		if a.isCaasAgent {
			manifolds = caasMachineManifolds(manifoldsCfg)
		}
		if err := dependency.Install(engine, manifolds); err != nil {
			if err := worker.Stop(engine); err != nil {
				logger.Errorf("while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		if err := addons.StartIntrospection(addons.IntrospectionConfig{
			AgentTag:           a.CurrentConfig().Tag(),
			Engine:             engine,
			StatePoolReporter:  &statePoolReporter,
			PubSubReporter:     pubsubReporter,
			MachineLock:        a.machineLock,
			NewSocketName:      a.newIntrospectionSocketName,
			PrometheusGatherer: a.prometheusRegistry,
			PresenceRecorder:   presenceRecorder,
			WorkerFunc:         introspection.NewWorker,
			Clock:              clock.WallClock,
			LocalHub:           localHub,
			CentralHub:         a.centralHub,
			LeaseFSM:           manifoldsCfg.LeaseFSM,
		}); err != nil {
			// If the introspection worker failed to start, we just log error
			// but continue. It is very unlikely to happen in the real world
			// as the only issue is connecting to the abstract domain socket
			// and the agent is controlled by by the OS to only have one.
			logger.Errorf("failed to start introspection worker: %v", err)
		}
		if err := addons.RegisterEngineMetrics(a.prometheusRegistry, metrics, engine, controllerMetricsSink); err != nil {
			// If the dependency engine metrics fail, continue on. This is unlikely
			// to happen in the real world, but should't stop or bring down an
			// agent.
			logger.Errorf("failed to start the dependency engine metrics %v", err)
		}
		return engine, nil
	}
}

func (a *MachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(a.CurrentConfig())
	if err != nil {
		return errors.Trace(err)
	}

	logger.Infof("Reboot: Executing reboot")
	err = finalize.ExecuteReboot(action)
	if err != nil {
		logger.Infof("Reboot: Error executing reboot: %v", err)
		return errors.Trace(err)
	}
	// On windows, the shutdown command is asynchronous. We return ErrRebootMachine
	// so the agent will simply exit without error pending reboot/shutdown.
	return jworker.ErrRebootMachine
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

// startAPIWorkers is called to start workers which rely on the
// machine agent's API connection (via the apiworkers manifold). It
// returns a Runner with a number of workers attached to it.
//
// The workers started here need to be converted to run under the
// dependency engine. Once they have all been converted, this method -
// and the apiworkers manifold - can be removed.
func (a *MachineAgent) startAPIWorkers(apiConn api.Connection) (_ worker.Worker, outErr error) {
	// CAAS agents do not have any api workers.
	if a.isCaasAgent {
		return nil, dependency.ErrUninstall
	}
	agentConfig := a.CurrentConfig()

	apiSt, err := apiagent.NewState(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	entity, err := apiSt.Entity(a.Tag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:       agenterrors.ConnectionIsFatal(logger, apiConn),
		MoreImportant: agenterrors.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})
	defer func() {
		// If startAPIWorkers exits early with an error, stop the
		// runner so that any already started runners aren't leaked.
		if outErr != nil {
			_ = worker.Stop(runner)
		}
	}()

	// Perform the operations needed to set up hosting for containers.
	if err := a.setupContainerSupport(runner, apiConn, agentConfig); err != nil {
		cause := errors.Cause(err)
		if params.IsCodeDead(cause) || cause == jworker.ErrTerminateAgent {
			return nil, jworker.ErrTerminateAgent
		}
		return nil, errors.Annotate(err, "setting up container support")
	}

	// Report the machine host name and record the agent start time. This
	// ensures that whenever a machine restarts, the instancepoller gets a
	// chance to immediately refresh the provider address (inc. shadow IP)
	// information which can change between reboots.
	hostname, err := getHostname()
	if err != nil {
		return nil, errors.Annotate(err, "getting machine hostname")
	}
	if err := a.recordAgentStartInformation(apiConn, hostname); err != nil {
		return nil, errors.Annotate(err, "recording agent start information")
	}

	var isController bool
	for _, job := range entity.Jobs() {
		switch job {
		case coremodel.JobManageModel:
			isController = true
		default:
			// TODO(dimitern): Once all workers moved over to using
			// the API, report "unknown job type" here.
		}
	}
	if !isController {
		_ = runner.StartWorker("stateconverter", func() (worker.Worker, error) {
			// TODO(fwereade): this worker needs its own facade.
			facade := apimachiner.NewState(apiConn)
			handler := conv2state.New(facade, a)
			w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
				Handler: handler,
			})
			if err != nil {
				return nil, errors.Annotate(err, "cannot start controller promoter worker")
			}
			return w, nil
		})
	}
	return runner, nil
}

type noopStatusSetter struct{}

// SetStatus implements upgradesteps.StatusSetter
func (a *noopStatusSetter) SetStatus(_ status.Status, _ string, _ map[string]interface{}) error {
	return nil
}

func (a *MachineAgent) statusSetter(apiConn api.Connection) (upgradesteps.StatusSetter, error) {
	if a.isCaasAgent || a.agentTag.Kind() != names.MachineTagKind {
		// TODO - support set status for controller agents
		return &noopStatusSetter{}, nil
	}
	machinerAPI := apimachiner.NewState(apiConn)
	return machinerAPI.Machine(a.Tag().(names.MachineTag))
}

func (a *MachineAgent) machine(apiConn api.Connection) (*apimachiner.Machine, error) {
	machinerAPI := apimachiner.NewState(apiConn)
	agentConfig := a.CurrentConfig()

	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("%q is not a machine tag", agentConfig.Tag().String())
	}
	return machinerAPI.Machine(tag)
}

func (a *MachineAgent) recordAgentStartInformation(apiConn api.Connection, hostname string) error {
	m, err := a.machine(apiConn)
	if errors.IsNotFound(err) || err == nil && m.Life() == life.Dead {
		return jworker.ErrTerminateAgent
	}
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", a.CurrentConfig().Tag())
	}

	if err := m.RecordAgentStartInformation(hostname); err != nil {
		return errors.Annotate(err, "cannot record agent start information")
	}
	return nil
}

// Restart restarts the agent's service.
func (a *MachineAgent) Restart() error {
	// TODO(bootstrap): revisit here to make it only invoked by IAAS.
	name := a.CurrentConfig().Value(agent.AgentServiceName)
	return service.Restart(name)
}

// openStateForUpgrade exists to be passed into the upgradesteps
// worker. The upgradesteps worker opens state independently of the
// state worker so that it isn't affected by the state worker's
// lifetime. It ensures the MongoDB server is configured and started,
// and then opens a state connection.
//
// TODO(mjs)- review the need for this once the dependency engine is
// in use. Why can't upgradesteps depend on the main state connection?
func (a *MachineAgent) openStateForUpgrade() (*state.StatePool, error) {
	agentConfig := a.CurrentConfig()
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, errors.Trace(err)
	}
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.New("no state info available")
	}
	dialOpts, err := mongoDialOptions(
		mongo.DefaultDialOpts(),
		agentConfig,
		a.mongoDialCollector,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	session, err := mongo.DialWithInfo(*info, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer session.Close()

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:              clock.WallClock,
		ControllerTag:      agentConfig.Controller(),
		ControllerModelTag: agentConfig.Model(),
		MongoSession:       session,
		NewPolicy:          stateenvirons.GetNewPolicyFunc(),
		// state.InitDatabase is idempotent and needs to be called just
		// prior to performing any upgrades since a new Juju binary may
		// declare new indices or explicit collections.
		// NB until https://jira.mongodb.org/browse/SERVER-1864 is resolved,
		// it is not possible to resize capped collections so there's no
		// point in reading existing controller config from state in order
		// to pass in the max-txn-log-size value.
		InitDatabaseFunc:       state.InitDatabase,
		RunTransactionObserver: a.mongoTxnCollector.AfterRunTransaction,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return pool, nil
}

// validateMigration is called by the migrationminion to help check
// that the agent will be ok when connected to a new controller.
func (a *MachineAgent) validateMigration(apiCaller base.APICaller) error {
	// TODO(mjs) - more extensive checks to come.
	var err error
	// TODO(controlleragent) - add k8s controller check.
	if !a.isCaasAgent {
		facade := apimachiner.NewState(apiCaller)
		_, err = facade.Machine(a.agentTag.(names.MachineTag))
	}
	return errors.Trace(err)
}

// setupContainerSupport determines what containers can be run on this machine and
// initialises suitable infrastructure to support such containers.
func (a *MachineAgent) setupContainerSupport(runner *worker.Runner, st api.Connection, agentConfig agent.Config) error {
	var supportedContainers []instance.ContainerType
	supportsContainers := container.ContainersSupported()
	if supportsContainers {
		supportedContainers = append(supportedContainers, instance.LXD)
	}

	supportsKvm, err := kvm.IsKVMSupported()
	if err != nil {
		logger.Warningf("determining kvm support: %v\nno kvm containers possible", err)
	}
	if err == nil && supportsKvm {
		supportedContainers = append(supportedContainers, instance.KVM)
	}

	return a.updateSupportedContainers(runner, st, supportedContainers, agentConfig)
}

// updateSupportedContainers records in state that a machine can run the specified containers.
// It starts a watcher and when a container of a given type is first added to the machine,
// the watcher is killed, the machine is set up to be able to start containers of the given type,
// and a suitable provisioner is started.
func (a *MachineAgent) updateSupportedContainers(
	runner *worker.Runner,
	st api.Connection,
	containers []instance.ContainerType,
	agentConfig agent.Config,
) error {
	pr := apiprovisioner.NewState(st)
	tag := agentConfig.Tag().(names.MachineTag)
	result, err := pr.Machines(tag)
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", tag)
	}
	if len(result) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(result))
	}
	if errors.IsNotFound(result[0].Err) || (result[0].Err == nil && result[0].Machine.Life() == life.Dead) {
		return jworker.ErrTerminateAgent
	}

	m := result[0].Machine
	if len(containers) == 0 {
		if err := m.SupportsNoContainers(); err != nil {
			return errors.Annotatef(err, "clearing supported containers for %s", tag)
		}
		return nil
	}
	if err := m.SetSupportedContainers(containers...); err != nil {
		return errors.Annotatef(err, "setting supported containers for %s", tag)
	}

	// Start the watcher to fire when a container is first requested on the machine.
	watcherName := fmt.Sprintf("%s-container-watcher", m.Id())

	credentialAPI, err := workercommon.NewCredentialInvalidatorFacade(st)
	if err != nil {
		return errors.Annotatef(err, "cannot get credential invalidator facade")
	}
	handler := provisioner.NewContainerSetupHandler(provisioner.ContainerSetupParams{
		Runner:              runner,
		Logger:              loggo.GetLogger("juju.container-setup"),
		WorkerName:          watcherName,
		SupportedContainers: containers,
		Machine:             m,
		Provisioner:         pr,
		Config:              agentConfig,
		MachineLock:         a.machineLock,
		CredentialAPI:       credentialAPI,
	})
	a.startWorkerAfterUpgrade(runner, watcherName, func() (worker.Worker, error) {
		w, err := watcher.NewStringsWorker(watcher.StringsConfig{
			Handler: handler,
		})
		if err != nil {
			return nil, errors.Annotatef(err, "cannot start %s worker", watcherName)
		}
		return w, nil
	})
	return nil
}

func mongoDialOptions(
	baseOpts mongo.DialOpts,
	agentConfig agent.Config,
	mongoDialCollector *mongometrics.DialCollector,
) (mongo.DialOpts, error) {
	dialOpts := baseOpts
	if limitStr := agentConfig.Value("MONGO_SOCKET_POOL_LIMIT"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return mongo.DialOpts{}, errors.Errorf("invalid mongo socket pool limit %q", limitStr)
		}
		logger.Infof("using mongo socker pool limit = %d", limit)
		dialOpts.PoolLimit = limit
	}
	if dialOpts.PostDialServer != nil {
		return mongo.DialOpts{}, errors.New("did not expect PostDialServer to be set")
	}
	dialOpts.PostDialServer = mongoDialCollector.PostDialServer
	return dialOpts, nil
}

func (a *MachineAgent) initController(agentConfig agent.Config) (*state.Controller, error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.Errorf("no state info available")
	}

	// Start MongoDB server and dial.
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}
	dialOpts, err := mongoDialOptions(
		stateWorkerDialOpts,
		agentConfig,
		a.mongoDialCollector,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	session, err := mongo.DialWithInfo(*info, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer session.Close()

	ctrl, err := state.OpenController(state.OpenParams{
		Clock:                  clock.WallClock,
		ControllerTag:          agentConfig.Controller(),
		ControllerModelTag:     agentConfig.Model(),
		MongoSession:           session,
		NewPolicy:              stateenvirons.GetNewPolicyFunc(),
		RunTransactionObserver: a.mongoTxnCollector.AfterRunTransaction,
	})
	return ctrl, errors.Trace(err)
}

func (a *MachineAgent) initState(agentConfig agent.Config) (*state.StatePool, error) {
	// Start MongoDB server and dial.
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}

	dialOpts, err := mongoDialOptions(
		stateWorkerDialOpts,
		agentConfig,
		a.mongoDialCollector,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pool, err := openStatePool(
		agentConfig,
		dialOpts,
		a.mongoTxnCollector.AfterRunTransaction,
	)
	if err != nil {
		// On error, force a mongo refresh.
		a.mongoInitMutex.Lock()
		a.mongoInitialized = false
		a.mongoInitMutex.Unlock()
		return nil, err
	}
	logger.Infof("juju database opened")

	reportOpenedState(pool.SystemState())

	return pool, nil
}

// startModelWorkers starts the set of workers that run for every model
// in each controller, both IAAS and CAAS.
func (a *MachineAgent) startModelWorkers(cfg modelworkermanager.NewModelConfig) (worker.Worker, error) {
	currentConfig := a.CurrentConfig()
	controllerUUID := currentConfig.Controller().Id()
	// We look at the model in the agent.conf file to see if we are starting workers
	// for our model.
	agentModelUUID := currentConfig.Model().Id()
	modelAgent, err := model.WrapAgent(a, controllerUUID, cfg.ModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	config := engine.DependencyEngineConfig(cfg.ModelMetrics)
	config.IsFatal = model.IsFatal
	config.WorstError = model.WorstError
	config.Filter = model.IgnoreErrRemoved
	engine, err := dependency.NewEngine(config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Send model logging to a different file on disk to the controller logging
	// for all models except the controller model.
	loggingContext := loggo.NewContext(loggo.INFO)
	var writer io.Writer
	if agentModelUUID == cfg.ModelUUID {
		writer = a.ctx.Stderr
	} else {
		modelsDir := filepath.Join(currentConfig.LogDir(), "models")
		if err := os.MkdirAll(modelsDir, 0755); err != nil {
			return nil, errors.Annotate(err, "unable to create models log directory")
		}
		if err := paths.SetSyslogOwner(modelsDir); err != nil {
			return nil, errors.Annotate(err, "unable to set owner for log directory")
		}
		filename := cfg.ModelName + "-" + cfg.ModelUUID[:6] + ".log"
		logFilename := filepath.Join(modelsDir, filename)
		if err := paths.PrimeLogFile(logFilename); err != nil {
			return nil, errors.Annotate(err, "unable to prime log file")
		}
		ljLogger := &lumberjack.Logger{
			Filename:   logFilename,
			MaxSize:    cfg.ControllerConfig.ModelLogfileMaxSizeMB(),
			MaxBackups: cfg.ControllerConfig.ModelLogfileMaxBackups(),
			Compress:   true,
		}
		logger.Debugf("created rotating log file %q with max size %d MB and max backups %d",
			ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
		writer = ljLogger
	}
	if err := loggingContext.AddWriter(
		"file", loggo.NewSimpleWriter(writer, loggo.DefaultFormatter)); err != nil {
		logger.Errorf("unable to configure file logging for model: %v", err)
	}
	// Use a standard state logger for the right model.
	if err := loggingContext.AddWriter("db", cfg.ModelLogger); err != nil {
		logger.Errorf("unable to configure db logging for model: %v", err)
	}

	manifoldsCfg := model.ManifoldsConfig{
		Agent:                       modelAgent,
		AgentConfigChanged:          a.configChangedVal,
		Authority:                   cfg.Authority,
		Clock:                       clock.WallClock,
		LoggingContext:              loggingContext,
		RunFlagDuration:             time.Minute,
		CharmRevisionUpdateInterval: 24 * time.Hour,
		StatusHistoryPrunerInterval: 5 * time.Minute,
		ActionPrunerInterval:        24 * time.Hour,
		Mux:                         cfg.Mux,
		NewEnvironFunc:              newEnvirons,
		NewContainerBrokerFunc:      newCAASBroker,
		NewMigrationMaster:          migrationmaster.NewWorker,
	}
	if wrench.IsActive("charmrevision", "shortinterval") {
		interval := 10 * time.Second
		logger.Infof("setting short charmrevision worker interval: %v", interval)
		manifoldsCfg.CharmRevisionUpdateInterval = interval
	}

	applyTestingOverrides(currentConfig, &manifoldsCfg)

	var manifolds dependency.Manifolds
	if cfg.ModelType == state.ModelTypeIAAS {
		manifolds = iaasModelManifolds(manifoldsCfg)
	} else {
		manifolds = caasModelManifolds(manifoldsCfg)
	}
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, errors.Trace(err)
	}

	return &modelWorker{
		Engine:    engine,
		logger:    cfg.ModelLogger,
		modelUUID: cfg.ModelUUID,
		metrics:   cfg.ModelMetrics,
	}, nil
}

func applyTestingOverrides(agentConfig agent.Config, manifoldsCfg *model.ManifoldsConfig) {
	if v := agentConfig.Value(agent.CharmRevisionUpdateInterval); v != "" {
		charmRevisionUpdateInterval, err := time.ParseDuration(v)
		if err == nil {
			manifoldsCfg.CharmRevisionUpdateInterval = charmRevisionUpdateInterval
			logger.Infof("model worker charm revision update interval set to %v for testing",
				charmRevisionUpdateInterval)
		} else {
			logger.Warningf("invalid charm revision update interval, using default %v: %v",
				manifoldsCfg.CharmRevisionUpdateInterval, err)
		}
	}
}

type modelWorker struct {
	*dependency.Engine
	logger    modelworkermanager.ModelLogger
	modelUUID string
	metrics   engine.MetricSink
}

// Wait is the last thing that is called on the worker as it is being
// removed.
func (m *modelWorker) Wait() error {
	err := m.Engine.Wait()

	logger.Debugf("closing db logger for %q", m.modelUUID)
	_ = m.logger.Close()
	// When closing the model, ensure that we also close the metrics with the
	// logger.
	_ = m.metrics.Unregister()
	return err
}

// stateWorkerDialOpts is a mongo.DialOpts suitable
// for use by StateWorker to dial mongo.
//
// This must be overridden in tests, as it assumes
// journaling is enabled.
var stateWorkerDialOpts mongo.DialOpts

// ensureMongoServer ensures that mongo is installed and running,
// and ready for opening a state connection.
func (a *MachineAgent) ensureMongoServer(agentConfig agent.Config) (err error) {
	a.mongoInitMutex.Lock()
	defer a.mongoInitMutex.Unlock()
	if a.mongoInitialized {
		logger.Debugf("mongo is already initialized")
		return nil
	}
	defer func() {
		if err == nil {
			a.mongoInitialized = true
		}
	}()

	if a.isCaasAgent {
		return nil
	}
	// EnsureMongoServer installs/upgrades the init config as necessary.
	ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	var mongodVersion mongo.Version
	if mongodVersion, err = cmdutil.EnsureMongoServer(ensureServerParams); err != nil {
		return err
	}
	logger.Debugf("mongodb service is installed")
	// update Mongo version.
	if err = a.ChangeConfig(
		func(config agent.ConfigSetter) error {
			config.SetMongoVersion(mongodVersion)
			return nil
		},
	); err != nil {
		return errors.Annotate(err, "cannot set mongo version")
	}
	return nil
}

func openStatePool(
	agentConfig agent.Config,
	dialOpts mongo.DialOpts,
	runTransactionObserver state.RunTransactionObserverFunc,
) (_ *state.StatePool, err error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.Errorf("no state info available")
	}
	session, err := mongo.DialWithInfo(*info, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer session.Close()

	pool, err := state.OpenStatePool(state.OpenParams{
		Clock:                  clock.WallClock,
		ControllerTag:          agentConfig.Controller(),
		ControllerModelTag:     agentConfig.Model(),
		MongoSession:           session,
		NewPolicy:              stateenvirons.GetNewPolicyFunc(),
		RunTransactionObserver: runTransactionObserver,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			pool.Close()
		}
	}()
	st := pool.SystemState()
	controller, err := st.FindEntity(agentConfig.Tag())
	if err != nil {
		if errors.IsNotFound(err) {
			err = jworker.ErrTerminateAgent
		}
		return nil, err
	}

	// Only machines (not controller agents) need to be provisioned.
	// TODO(controlleragent) - this needs to be reworked
	m, ok := controller.(*state.Machine)
	if !ok {
		return pool, err
	}
	if m.Life() == state.Dead {
		return nil, jworker.ErrTerminateAgent
	}
	// Check the machine nonce as provisioned matches the agent.Conf value.
	if !m.CheckProvisioned(agentConfig.Nonce()) {
		// The agent is running on a different machine to the one it
		// should be according to state. It must stop immediately.
		logger.Errorf("running machine %v agent on inappropriate instance", m)
		return nil, jworker.ErrTerminateAgent
	}
	return pool, nil
}

// startWorkerAfterUpgrade starts a worker to run the specified child worker
// but only after waiting for upgrades to complete.
func (a *MachineAgent) startWorkerAfterUpgrade(runner jworker.Runner, name string, start func() (worker.Worker, error)) {
	_ = runner.StartWorker(name, func() (worker.Worker, error) {
		return a.upgradeWaiterWorker(name, start), nil
	})
}

// upgradeWaiterWorker runs the specified worker after upgrades have completed.
func (a *MachineAgent) upgradeWaiterWorker(name string, start func() (worker.Worker, error)) worker.Worker {
	return jworker.NewSimpleWorker(func(stop <-chan struct{}) error {
		// Wait for the agent upgrade and upgrade steps to complete (or for us to be stopped).
		for _, ch := range []<-chan struct{}{
			a.upgradeComplete.Unlocked(),
			a.initialUpgradeCheckComplete.Unlocked(),
		} {
			select {
			case <-stop:
				return nil
			case <-ch:
			}
		}
		logger.Debugf("upgrades done, starting worker %q", name)

		// Upgrades are done, start the worker.
		w, err := start()
		if err != nil {
			return err
		}
		// Wait for worker to finish or for us to be stopped.
		done := make(chan error, 1)
		go func() {
			done <- w.Wait()
		}()
		select {
		case err := <-done:
			return errors.Annotatef(err, "worker %q exited", name)
		case <-stop:
			logger.Debugf("stopping so killing worker %q", name)
			return worker.Stop(w)
		}
	})
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

	currentTarget, err := symlink.Read(fullLink)
	if err != nil && !os.IsNotExist(err) {
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
