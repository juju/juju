// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/pubsub"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	utilscert "github.com/juju/utils/cert"
	"github.com/juju/utils/symlink"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	apideployer "github.com/juju/juju/api/deployer"
	apimachiner "github.com/juju/juju/api/machiner"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/cmd/jujud/reboot"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongometrics"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/upgrades"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
	workercommon "github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/conv2state"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/introspection"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/logsender/logsendermetrics"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/provisioner"
	psworker "github.com/juju/juju/worker/pubsub"
	"github.com/juju/juju/worker/upgradesteps"
)

var (
	logger           = loggo.GetLogger("juju.cmd.jujud")
	jujuRun          = paths.MustSucceed(paths.JujuRun(series.MustHostSeries()))
	jujuDumpLogs     = paths.MustSucceed(paths.JujuDumpLogs(series.MustHostSeries()))
	jujuIntrospect   = paths.MustSucceed(paths.JujuIntrospect(series.MustHostSeries()))
	jujuUpdateSeries = paths.MustSucceed(paths.JujuUpdateSeries(series.MustHostSeries()))
	jujudSymlinks    = []string{jujuRun, jujuDumpLogs, jujuIntrospect, jujuUpdateSeries}

	// The following are defined as variables to allow the tests to
	// intercept calls to the functions. In every case, they should
	// be expressed as explicit dependencies, but nobody has yet had
	// the intestinal fortitude to untangle this package. Be that
	// person! Juju Needs You.
	useMultipleCPUs   = utils.UseMultipleCPUs
	reportOpenedState = func(*state.State) {}

	caasModelManifolds = model.CAASManifolds
	iaasModelManifolds = model.IAASManifolds
	machineManifolds   = machine.Manifolds
)

// Variable to override in tests, default is true
var ProductionMongoWriteConcern = true

func init() {
	stateWorkerDialOpts = mongo.DefaultDialOpts()
	stateWorkerDialOpts.PostDial = func(session *mgo.Session) error {
		logger.Criticalf("cmd/jujud/agent/machine.go  init -> %#v", session)
		safe := mgo.Safe{}
		if ProductionMongoWriteConcern {
			safe.J = true
			_, err := replicaset.CurrentConfig(session)
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

// AgentInitializer handles initializing a type for use as a Jujud
// agent.
type AgentInitializer interface {
	AddFlags(*gnuflag.FlagSet)
	CheckArgs([]string) error
}

// AgentConfigWriter encapsulates disk I/O operations with the agent
// config.
type AgentConfigWriter interface {
	// ReadConfig reads the config for the given tag from disk.
	ReadConfig(tag string) error
	// ChangeConfig executes the given agent.ConfigMutator in a
	// thread-safe context.
	ChangeConfig(agent.ConfigMutator) error
	// CurrentConfig returns a copy of the in-memory agent config.
	CurrentConfig() agent.Config
}

// NewMachineAgentCmd creates a Command which handles parsing
// command-line arguments and instantiating and running a
// MachineAgent.
func NewMachineAgentCmd(
	ctx *cmd.Context,
	machineAgentFactory func(string) (*MachineAgent, error),
	agentInitializer AgentInitializer,
	configFetcher AgentConfigWriter,
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
	currentConfig       AgentConfigWriter
	machineAgentFactory func(string) (*MachineAgent, error)
	ctx                 *cmd.Context

	// This group is for debugging purposes.
	logToStdErr bool

	// The following are set via command-line flags.
	machineId string
}

// Init is called by the cmd system to initialize the structure for
// running.
func (a *machineAgentCmd) Init(args []string) error {

	if !names.IsValidMachine(a.machineId) {
		return errors.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	if err := a.agentInitializer.CheckArgs(args); err != nil {
		return err
	}

	// Due to changes in the logging, and needing to care about old
	// models that have been upgraded, we need to explicitly remove the
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	loggo.RemoveWriter("logfile")

	if a.logToStdErr {
		return nil
	}

	err := a.currentConfig.ReadConfig(names.NewMachineTag(a.machineId).String())
	if err != nil {
		return errors.Annotate(err, "cannot read agent configuration")
	}

	config := a.currentConfig.CurrentConfig()
	// the context's stderr is set as the loggo writer in github.com/juju/cmd/logging.go
	a.ctx.Stderr = &lumberjack.Logger{
		Filename:   agent.LogFilename(config),
		MaxSize:    300, // megabytes
		MaxBackups: 2,
		Compress:   true,
	}

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *machineAgentCmd) Run(c *cmd.Context) error {
	machineAgent, err := a.machineAgentFactory(a.machineId)
	if err != nil {
		return errors.Trace(err)
	}
	return machineAgent.Run(c)
}

// SetFlags adds the requisite flags to run this command.
func (a *machineAgentCmd) SetFlags(f *gnuflag.FlagSet) {
	a.agentInitializer.AddFlags(f)
	f.StringVar(&a.machineId, "machine-id", "", "id of the machine to run")
}

// Info returns usage information for the command.
func (a *machineAgentCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "machine",
		Purpose: "run a juju machine agent",
	}
}

// MachineAgentFactoryFn returns a function which instantiates a
// MachineAgent given a machineId.
func MachineAgentFactoryFn(
	agentConfWriter AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	newIntrospectionSocketName func(names.Tag) string,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	rootDir string,
) func(string) (*MachineAgent, error) {
	return func(machineId string) (*MachineAgent, error) {
		return NewMachineAgent(
			machineId,
			agentConfWriter,
			bufferedLogger,
			worker.NewRunner(worker.RunnerParams{
				IsFatal:       cmdutil.IsFatal,
				MoreImportant: cmdutil.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
			}),
			looputil.NewLoopDeviceManager(),
			newIntrospectionSocketName,
			preUpgradeSteps,
			rootDir,
		)
	}
}

// NewMachineAgent instantiates a new MachineAgent.
func NewMachineAgent(
	machineId string,
	agentConfWriter AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
	runner *worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
	newIntrospectionSocketName func(names.Tag) string,
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	rootDir string,
) (*MachineAgent, error) {
	prometheusRegistry, err := newPrometheusRegistry()
	if err != nil {
		return nil, errors.Trace(err)
	}
	a := &MachineAgent{
		machineId:                   machineId,
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
	}
	if err := a.registerPrometheusCollectors(); err != nil {
		return nil, errors.Trace(err)
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
		logsendermetrics.BufferedLogWriterMetrics{a.bufferedLogger},
	); err != nil {
		return errors.Annotate(err, "registering logsender collector")
	}
	if err := a.prometheusRegistry.Register(a.mongoTxnCollector); err != nil {
		return errors.Annotate(err, "registering mgo/txn collector")
	}
	if err := a.prometheusRegistry.Register(a.mongoDialCollector); err != nil {
		return errors.Annotate(err, "registering mongo dial collector")
	}
	return nil
}

// MachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type MachineAgent struct {
	AgentConfigWriter

	dead             chan struct{}
	errReason        error
	machineId        string
	runner           *worker.Runner
	rootDir          string
	bufferedLogger   *logsender.BufferedLogWriter
	configChangedVal *voyeur.Value
	upgradeComplete  gate.Lock
	workersStarted   chan struct{}
	machineLock      machinelock.Lock

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
	centralHub *pubsub.StructuredHub
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

	// Validate the current certificate and private key pair, and then
	// extract the current DNS names from the certificate. If the
	// certificate validation fails, or it does not contain the DNS
	// names we require, we will generate a new one.
	var dnsNames set.Strings
	serverCert, _, err := utilscert.ParseCertAndKey(si.Cert, si.PrivateKey)
	if err != nil {
		// The certificate is invalid, so create a new one.
		logger.Infof("parsing certificate/key failed, will generate a new one: %v", err)
		dnsNames = set.NewStrings()
	} else {
		dnsNames = set.NewStrings(serverCert.DNSNames...)
	}
	logger.Criticalf("upgradeCertificateDNSNames.dnsNames 1 -> %#v", dnsNames)
	update := false
	requiredDNSNames := []string{"localhost", "juju-apiserver", "juju-mongodb"}
	for _, dnsName := range requiredDNSNames {
		logger.Criticalf("upgradeCertificateDNSNames.dnsNames 2 -> %#v, dnsName -> %q", dnsNames, dnsName)
		if dnsNames.Contains(dnsName) {
			continue
		}
		dnsNames.Add(dnsName)
		update = true
	}
	logger.Criticalf("upgradeCertificateDNSNames.update -> %t", update)
	if !update {
		return nil
	}

	// Write a new certificate to the mongo pem and agent config files.
	si.Cert, si.PrivateKey, err = cert.NewDefaultServer(config.CACert(), si.CAPrivateKey, dnsNames.Values())
	logger.Criticalf("upgradeCertificateDNSNames update cert to mongo 1 !!!!")
	if err != nil {
		return err
	}
	logger.Criticalf("upgradeCertificateDNSNames update cert to mongo 2 !!!!")
	if err := mongo.UpdateSSLKey(config.DataDir(), si.Cert, si.PrivateKey); err != nil {
		return err
	}
	logger.Criticalf("upgradeCertificateDNSNames update cert to agent.conf file !!!!")
	config.SetStateServingInfo(si)
	return nil
}

// Run runs a machine agent.
func (a *MachineAgent) Run(*cmd.Context) (err error) {
	defer a.Done(err)
	useMultipleCPUs()
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return errors.Errorf("cannot read agent configuration: %v", err)
	}

	setupAgentLogging(a.CurrentConfig())

	// if err := introspection.WriteProfileFunctions(); err != nil {
	// 	// This isn't fatal, just annoying.
	// 	logger.Errorf("failed to write profile funcs: %v", err)
	// }

	// When the API server and peergrouper have manifolds, they can
	// have dependencies on a central hub worker.
	a.centralHub = centralhub.New(a.Tag().(names.MachineTag))

	// // Before doing anything else, we need to make sure the certificate generated for
	// // use by mongo to validate controller connections is correct. This needs to be done
	// // before any possible restart of the mongo service.
	// // See bug http://pad.lv/1434680
	// if err := a.AgentConfigWriter.ChangeConfig(upgradeCertificateDNSNames); err != nil {
	// 	return errors.Annotate(err, "error upgrading server certificate")
	// }

	agentConfig := a.CurrentConfig()
	machineLock, err := machinelock.New(machinelock.Config{
		AgentName:   a.Tag().String(),
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
	a.upgradeComplete = upgradesteps.NewLock(agentConfig)

	createEngine := a.makeEngineCreator(agentConfig.UpgradedToVersion())
	charmrepo.CacheDir = filepath.Join(agentConfig.DataDir(), "charmcache")
	if err := a.createJujudSymlinks(agentConfig.DataDir()); err != nil {
		return err
	}
	a.runner.StartWorker("engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err = a.runner.Wait()
	switch errors.Cause(err) {
	case jworker.ErrTerminateAgent:
		err = a.uninstallAgent()
	case jworker.ErrRebootMachine:
		logger.Infof("Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case jworker.ErrShutdownMachine:
		logger.Infof("Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	return cmdutil.AgentDone(logger, err)
}

func (a *MachineAgent) makeEngineCreator(previousAgentVersion version.Number) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		config := dependency.EngineConfig{
			IsFatal:     cmdutil.IsFatal,
			WorstError:  cmdutil.MoreImportantError,
			ErrorDelay:  3 * time.Second,
			BounceDelay: 10 * time.Millisecond,
			Clock:       clock.WallClock,
			Logger:      loggo.GetLogger("juju.worker.dependency"),
		}
		engine, err := dependency.NewEngine(config)
		if err != nil {
			return nil, err
		}
		pubsubReporter := psworker.NewReporter()
		presenceRecorder := presence.New(clock.WallClock)
		updateAgentConfLogging := func(loggingConfig string) error {
			return a.AgentConfigWriter.ChangeConfig(func(setter agent.ConfigSetter) error {
				setter.SetLoggingConfig(loggingConfig)
				return nil
			})
		}

		// statePoolReporter is an introspection.IntrospectionReporter,
		// which is set to the current StatePool managed by the state
		// tracker in controller agents.
		var statePoolReporter statePoolIntrospectionReporter
		registerIntrospectionHandlers := func(handle func(path string, h http.Handler)) {
			introspection.RegisterHTTPHandlers(introspection.ReportSources{
				DependencyEngine:   engine,
				StatePool:          &statePoolReporter,
				PubSub:             pubsubReporter,
				PrometheusGatherer: a.prometheusRegistry,
			}, handle)
		}

		// We need to pass this in for the peergrouper, which wants to
		// know whether the controller model supports spaces.
		//
		// TODO(axw) this seems unnecessary, and perhaps even wrong.
		// Even if the provider supports spaces, you could have manual
		// machines in the mix, in which case they won't necessarily
		// be in the same space. I think the peergrouper should just
		// check what spaces the machines are in, rather than trying
		// to short cut anything.
		controllerSupportsSpaces := func(st *state.State) (bool, error) {
			env, err := stateenvirons.GetNewEnvironFunc(environs.New)(st)
			if err != nil {
				return false, errors.Annotate(err, "getting environ from state")
			}
			return environs.SupportsSpaces(state.CallContext(st), env), nil
		}

		manifolds := machineManifolds(machine.ManifoldsConfig{
			PreviousAgentVersion: previousAgentVersion,
			Agent:                agent.APIHostPortsSetter{Agent: a},
			RootDir:              a.rootDir,
			AgentConfigChanged:   a.configChangedVal,
			UpgradeStepsLock:     a.upgradeComplete,
			UpgradeCheckLock:     a.initialUpgradeCheckComplete,
			OpenController:       a.initController,
			OpenStatePool:        a.initState,
			OpenStateForUpgrade:  a.openStateForUpgrade,
			StartAPIWorkers:      a.startAPIWorkers,
			PreUpgradeSteps:      a.preUpgradeSteps,
			LogSource:            a.bufferedLogger.Logs(),
			NewDeployContext:     newDeployContext,
			Clock:                clock.WallClock,
			ValidateMigration:    a.validateMigration,
			PrometheusRegisterer: a.prometheusRegistry,
			CentralHub:           a.centralHub,
			PubSubReporter:       pubsubReporter,
			PresenceRecorder:     presenceRecorder,
			UpdateLoggerConfig:   updateAgentConfLogging,
			NewAgentStatusSetter: func(apiConn api.Connection) (upgradesteps.StatusSetter, error) {
				return a.machine(apiConn)
			},
			ControllerLeaseDuration:           time.Minute,
			LogPruneInterval:                  5 * time.Minute,
			TransactionPruneInterval:          time.Hour,
			MachineLock:                       a.machineLock,
			SetStatePool:                      statePoolReporter.set,
			RegisterIntrospectionHTTPHandlers: registerIntrospectionHandlers,
			NewModelWorker:                    a.startModelWorkers,
			ControllerSupportsSpaces:          controllerSupportsSpaces,
		})
		if err := dependency.Install(engine, manifolds); err != nil {
			if err := worker.Stop(engine); err != nil {
				logger.Errorf("while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		if err := startIntrospection(introspectionConfig{
			Agent:              a,
			Engine:             engine,
			StatePoolReporter:  &statePoolReporter,
			PubSubReporter:     pubsubReporter,
			MachineLock:        a.machineLock,
			NewSocketName:      a.newIntrospectionSocketName,
			PrometheusGatherer: a.prometheusRegistry,
			PresenceRecorder:   presenceRecorder,
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
}

func (a *MachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// At this stage, all API connections would have been closed
	// We need to reopen the API to clear the reboot flag after
	// scheduling the reboot. It may be cleaner to do this in the reboot
	// worker, before returning the ErrRebootMachine.
	conn, err := apicaller.OnlyConnect(a, api.Open)
	if err != nil {
		logger.Infof("Reboot: Error connecting to state")
		return errors.Trace(err)
	}

	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(conn, a.CurrentConfig())
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
)

// startAPIWorkers is called to start workers which rely on the
// machine agent's API connection (via the apiworkers manifold). It
// returns a Runner with a number of workers attached to it.
//
// The workers started here need to be converted to run under the
// dependency engine. Once they have all been converted, this method -
// and the apiworkers manifold - can be removed.
func (a *MachineAgent) startAPIWorkers(apiConn api.Connection) (_ worker.Worker, outErr error) {
	agentConfig := a.CurrentConfig()

	apiSt, err := apiagent.NewState(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	entity, err := apiSt.Entity(a.Tag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var isModelManager bool
	for _, job := range entity.Jobs() {
		switch job {
		case multiwatcher.JobManageModel:
			isModelManager = true
		default:
			// TODO(dimitern): Once all workers moved over to using
			// the API, report "unknown job type" here.
		}
	}

	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:       cmdutil.ConnectionIsFatal(logger, apiConn),
		MoreImportant: cmdutil.MoreImportant,
		RestartDelay:  jworker.RestartDelay,
	})
	defer func() {
		// If startAPIWorkers exits early with an error, stop the
		// runner so that any already started runners aren't leaked.
		if outErr != nil {
			worker.Stop(runner)
		}
	}()

	// Perform the operations needed to set up hosting for containers.
	if err := a.setupContainerSupport(runner, apiConn, agentConfig); err != nil {
		cause := errors.Cause(err)
		if params.IsCodeDead(cause) || cause == jworker.ErrTerminateAgent {
			return nil, jworker.ErrTerminateAgent
		}
		return nil, errors.Errorf("setting up container support: %v", err)
	}

	if isModelManager {
		// We don't have instance info set and the network config for the
		// bootstrap machine only, so update it now. All the other machines will
		// have instance info including network config set at provisioning time.
		if err := a.setControllerNetworkConfig(apiConn); err != nil {
			return nil, errors.Annotate(err, "setting controller network config")
		}
	} else {
		runner.StartWorker("stateconverter", func() (worker.Worker, error) {
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

func (a *MachineAgent) machine(apiConn api.Connection) (*apimachiner.Machine, error) {
	machinerAPI := apimachiner.NewState(apiConn)
	agentConfig := a.CurrentConfig()

	tag := agentConfig.Tag().(names.MachineTag)
	return machinerAPI.Machine(tag)
}

func (a *MachineAgent) setControllerNetworkConfig(apiConn api.Connection) error {
	machine, err := a.machine(apiConn)
	if errors.IsNotFound(err) || err == nil && machine.Life() == params.Dead {
		return jworker.ErrTerminateAgent
	}
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", a.CurrentConfig().Tag())
	}

	if err := machine.SetProviderNetworkConfig(); err != nil {
		return errors.Annotate(err, "cannot set controller provider network config")
	}
	return nil
}

// Restart restarts the agent's service.
func (a *MachineAgent) Restart() error {
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
	facade := apimachiner.NewState(apiCaller)
	_, err := facade.Machine(names.NewMachineTag(a.machineId))
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
		return errors.Annotatef(err, "expected 1 result, got %d", len(result))
	}
	if errors.IsNotFound(result[0].Err) || (result[0].Err == nil && result[0].Machine.Life() == params.Dead) {
		return jworker.ErrTerminateAgent
	}
	machine := result[0].Machine
	if len(containers) == 0 {
		if err := machine.SupportsNoContainers(); err != nil {
			return errors.Annotatef(err, "clearing supported containers for %s", tag)
		}
		return nil
	}
	if err := machine.SetSupportedContainers(containers...); err != nil {
		return errors.Annotatef(err, "setting supported containers for %s", tag)
	}
	// Start the watcher to fire when a container is first requested on the machine.
	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())

	credentialAPI, err := workercommon.NewCredentialInvalidatorFacade(st)
	if err != nil {
		return errors.Annotatef(err, "cannot get credential invalidator facade")
	}
	params := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: containers,
		Machine:             machine,
		Provisioner:         pr,
		Config:              agentConfig,
		MachineLock:         a.machineLock,
		CredentialAPI:       credentialAPI,
	}
	handler := provisioner.NewContainerSetupHandler(params)
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
		} else {
			logger.Infof("using mongo socker pool limit = %d", limit)
			dialOpts.PoolLimit = limit
		}
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

	ctlr, err := state.OpenController(state.OpenParams{
		Clock:                  clock.WallClock,
		ControllerTag:          agentConfig.Controller(),
		ControllerModelTag:     agentConfig.Model(),
		MongoSession:           session,
		NewPolicy:              stateenvirons.GetNewPolicyFunc(),
		RunTransactionObserver: a.mongoTxnCollector.AfterRunTransaction,
	})
	return ctlr, nil
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
	pool, _, err := openStatePool(
		agentConfig,
		dialOpts,
		a.mongoTxnCollector.AfterRunTransaction,
	)
	if err != nil {
		return nil, err
	}

	reportOpenedState(pool.SystemState())

	return pool, nil
}

// startModelWorkers starts the set of workers that run for every model
// in each controller, both IAAS and CAAS.
func (a *MachineAgent) startModelWorkers(modelUUID string, modelType state.ModelType) (worker.Worker, error) {
	controllerUUID := a.CurrentConfig().Controller().Id()
	modelAgent, err := model.WrapAgent(a, controllerUUID, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	engine, err := dependency.NewEngine(dependency.EngineConfig{
		IsFatal:     model.IsFatal,
		WorstError:  model.WorstError,
		Filter:      model.IgnoreErrRemoved,
		ErrorDelay:  3 * time.Second,
		BounceDelay: 10 * time.Millisecond,
		Clock:       clock.WallClock,
		Logger:      loggo.GetLogger("juju.worker.dependency"),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifoldsCfg := model.ManifoldsConfig{
		Agent:                       modelAgent,
		AgentConfigChanged:          a.configChangedVal,
		Clock:                       clock.WallClock,
		RunFlagDuration:             time.Minute,
		CharmRevisionUpdateInterval: 24 * time.Hour,
		InstPollerAggregationDelay:  3 * time.Second,
		StatusHistoryPrunerInterval: 5 * time.Minute,
		ActionPrunerInterval:        24 * time.Hour,
		NewEnvironFunc:              newEnvirons,
		NewContainerBrokerFunc:      newCAASBroker,
		NewMigrationMaster:          migrationmaster.NewWorker,
	}
	var manifolds dependency.Manifolds
	if modelType == state.ModelTypeIAAS {
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
	return engine, nil
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

	// TODO: No need for k8s coz it's ensured in mongo container in same pod

	// // EnsureMongoServer installs/upgrades the init config as necessary.
	// ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	// if err != nil {
	// 	return err
	// }
	var mongodVersion mongo.Version
	// if mongodVersion, err = cmdutil.EnsureMongoServer(ensureServerParams); err != nil {
	// 	return err
	// }
	logger.Debugf("mongodb service is installed")

	// Mongo is installed, record the version.
	err = a.ChangeConfig(func(config agent.ConfigSetter) error {
		config.SetMongoVersion(mongodVersion)
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "cannot set mongo version")
	}
	return nil
}

func openStatePool(
	agentConfig agent.Config,
	dialOpts mongo.DialOpts,
	runTransactionObserver state.RunTransactionObserverFunc,
) (_ *state.StatePool, _ *state.Machine, err error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, nil, errors.Errorf("no state info available")
	}
	session, err := mongo.DialWithInfo(*info, dialOpts)
	if err != nil {
		return nil, nil, errors.Trace(err)
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
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			pool.Close()
		}
	}()
	st := pool.SystemState()
	m0, err := st.FindEntity(agentConfig.Tag())
	if err != nil {
		if errors.IsNotFound(err) {
			err = jworker.ErrTerminateAgent
		}
		return nil, nil, err
	}
	m := m0.(*state.Machine)
	if m.Life() == state.Dead {
		return nil, nil, jworker.ErrTerminateAgent
	}
	// Check the machine nonce as provisioned matches the agent.Conf value.
	if !m.CheckProvisioned(agentConfig.Nonce()) {
		// The agent is running on a different machine to the one it
		// should be according to state. It must stop immediately.
		logger.Errorf("running machine %v agent on inappropriate instance", m)
		return nil, nil, jworker.ErrTerminateAgent
	}
	return pool, m, nil
}

// startWorkerAfterUpgrade starts a worker to run the specified child worker
// but only after waiting for upgrades to complete.
func (a *MachineAgent) startWorkerAfterUpgrade(runner jworker.Runner, name string, start func() (worker.Worker, error)) {
	runner.StartWorker(name, func() (worker.Worker, error) {
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
	return names.NewMachineTag(a.machineId)
}

func (a *MachineAgent) createJujudSymlinks(dataDir string) error {
	jujud := filepath.Join(tools.ToolsDir(dataDir, a.Tag().String()), jujunames.Jujud)
	for _, link := range jujudSymlinks {
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

func (a *MachineAgent) removeJujudSymlinks() (errs []error) {
	for _, link := range jujudSymlinks {
		err := os.Remove(utils.EnsureBaseDir(a.rootDir, link))
		if err != nil && !os.IsNotExist(err) {
			errs = append(errs, errors.Annotatef(err, "failed to remove %s symlink", link))
		}
	}
	return
}

func (a *MachineAgent) uninstallAgent() error {
	// We should only uninstall if the uninstall file is present.
	if !agent.CanUninstall(a) {
		logger.Infof("ignoring uninstall request")
		return nil
	}
	logger.Infof("uninstalling agent")

	agentConfig := a.CurrentConfig()
	var errs []error
	agentServiceName := agentConfig.Value(agent.AgentServiceName)
	if agentServiceName == "" {
		// For backwards compatibility, handle lack of AgentServiceName.
		agentServiceName = os.Getenv("UPSTART_JOB")
	}

	if agentServiceName != "" {
		svc, err := service.DiscoverService(agentServiceName, common.Conf{})
		if err != nil {
			errs = append(errs, errors.Errorf("cannot remove service %q: %v", agentServiceName, err))
		} else if err := svc.Remove(); err != nil {
			errs = append(errs, errors.Errorf("cannot remove service %q: %v", agentServiceName, err))
		}
	}

	errs = append(errs, a.removeJujudSymlinks()...)

	// TODO(fwereade): surely this shouldn't be happening here? Once we're
	// at this point we should expect to be killed in short order; if this
	// work is remotely important we should be blocking machine death on
	// its completion.
	insideContainer := container.RunningInContainer()
	if insideContainer {
		// We're running inside a container, so loop devices may leak. Detach
		// any loop devices that are backed by files on this machine.
		if err := a.loopDeviceManager.DetachLoopDevices("/", agentConfig.DataDir()); err != nil {
			errs = append(errs, err)
		}
	}

	if err := mongo.RemoveService(); err != nil {
		errs = append(errs, errors.Annotate(err, "cannot stop/remove mongo service"))
	}
	if err := os.RemoveAll(agentConfig.DataDir()); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Errorf("uninstall failed: %v", errs)
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, agentConfig agent.Config) deployer.Context {
	return deployer.NewSimpleContext(agentConfig, st)
}
