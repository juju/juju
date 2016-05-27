// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	apiagent "github.com/juju/juju/api/agent"
	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
	"github.com/juju/utils/voyeur"
	"github.com/juju/version"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/cmd/jujud/reboot"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/conv2state"
	"github.com/juju/juju/worker/dblogpruner"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/imagemetadataworker"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/modelworkermanager"
	"github.com/juju/juju/worker/mongoupgrader"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/txnpruner"
	"github.com/juju/juju/worker/upgradesteps"
)

var (
	logger       = loggo.GetLogger("juju.cmd.jujud")
	jujuRun      = paths.MustSucceed(paths.JujuRun(series.HostSeries()))
	jujuDumpLogs = paths.MustSucceed(paths.JujuDumpLogs(series.HostSeries()))

	// The following are defined as variables to allow the tests to
	// intercept calls to the functions.
	useMultipleCPUs       = utils.UseMultipleCPUs
	modelManifolds        = model.Manifolds
	newSingularRunner     = singular.New
	peergrouperNew        = peergrouper.New
	newCertificateUpdater = certupdater.NewCertificateUpdater
	newMetadataUpdater    = imagemetadataworker.NewWorker
	newUpgradeMongoWorker = mongoupgrader.New
	reportOpenedState     = func(io.Closer) {}
)

// Variable to override in tests, default is true
var ProductionMongoWriteConcern = true

func init() {
	stateWorkerDialOpts = mongo.DefaultDialOpts()
	stateWorkerDialOpts.PostDial = func(session *mgo.Session) error {
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
	machineAgentFactory func(string) *MachineAgent,
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
	machineAgentFactory func(string) *MachineAgent
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
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
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

	// the context's stderr is set as the loggo writer in github.com/juju/cmd/logging.go
	a.ctx.Stderr = &lumberjack.Logger{
		Filename:   agent.LogFilename(a.currentConfig.CurrentConfig()),
		MaxSize:    300, // megabytes
		MaxBackups: 2,
	}

	return nil
}

// Run instantiates a MachineAgent and runs it.
func (a *machineAgentCmd) Run(c *cmd.Context) error {
	machineAgent := a.machineAgentFactory(a.machineId)
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
	bufferedLogs logsender.LogRecordCh,
	rootDir string,
) func(string) *MachineAgent {
	return func(machineId string) *MachineAgent {
		return NewMachineAgent(
			machineId,
			agentConfWriter,
			bufferedLogs,
			worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant, worker.RestartDelay),
			looputil.NewLoopDeviceManager(),
			rootDir,
		)
	}
}

// NewMachineAgent instantiates a new MachineAgent.
func NewMachineAgent(
	machineId string,
	agentConfWriter AgentConfigWriter,
	bufferedLogs logsender.LogRecordCh,
	runner worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
	rootDir string,
) *MachineAgent {
	return &MachineAgent{
		machineId:                   machineId,
		AgentConfigWriter:           agentConfWriter,
		configChangedVal:            voyeur.NewValue(true),
		bufferedLogs:                bufferedLogs,
		workersStarted:              make(chan struct{}),
		runner:                      runner,
		rootDir:                     rootDir,
		initialUpgradeCheckComplete: gate.NewLock(),
		loopDeviceManager:           loopDeviceManager,
	}
}

// MachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type MachineAgent struct {
	AgentConfigWriter

	tomb             tomb.Tomb
	machineId        string
	runner           worker.Runner
	rootDir          string
	bufferedLogs     logsender.LogRecordCh
	configChangedVal *voyeur.Value
	upgradeComplete  gate.Lock
	workersStarted   chan struct{}

	// XXX(fwereade): these smell strongly of goroutine-unsafeness.
	restoreMode bool
	restoring   bool

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	initialUpgradeCheckComplete gate.Lock

	discoverSpacesComplete gate.Lock

	mongoInitMutex   sync.Mutex
	mongoInitialized bool

	loopDeviceManager looputil.LoopDeviceManager
}

// IsRestorePreparing returns bool representing if we are in restore mode
// but not running restore.
func (a *MachineAgent) IsRestorePreparing() bool {
	return a.restoreMode && !a.restoring
}

// IsRestoreRunning returns bool representing if we are in restore mode
// and running the actual restore process.
func (a *MachineAgent) IsRestoreRunning() bool {
	return a.restoring
}

func (a *MachineAgent) isUpgradeRunning() bool {
	return !a.upgradeComplete.IsUnlocked()
}

func (a *MachineAgent) isInitialUpgradeCheckPending() bool {
	return !a.initialUpgradeCheckComplete.IsUnlocked()
}

// Wait waits for the machine agent to finish.
func (a *MachineAgent) Wait() error {
	return a.tomb.Wait()
}

// Stop stops the machine agent.
func (a *MachineAgent) Stop() error {
	a.runner.Kill()
	return a.tomb.Wait()
}

// upgradeCertificateDNSNames ensure that the controller certificate
// recorded in the agent config and also mongo server.pem contains the
// DNSNames entires required by Juju/
func (a *MachineAgent) upgradeCertificateDNSNames() error {
	agentConfig := a.CurrentConfig()
	si, ok := agentConfig.StateServingInfo()
	if !ok || si.CAPrivateKey == "" {
		// No certificate information exists yet, nothing to do.
		return nil
	}
	// Parse the current certificate to get the current dns names.
	serverCert, err := cert.ParseCert(si.Cert)
	if err != nil {
		return err
	}
	update := false
	dnsNames := set.NewStrings(serverCert.DNSNames...)
	requiredDNSNames := []string{"local", "juju-apiserver", "juju-mongodb"}
	for _, dnsName := range requiredDNSNames {
		if dnsNames.Contains(dnsName) {
			continue
		}
		dnsNames.Add(dnsName)
		update = true
	}
	if !update {
		return nil
	}
	// Write a new certificate to the mongo pem and agent config files.
	si.Cert, si.PrivateKey, err = cert.NewDefaultServer(agentConfig.CACert(), si.CAPrivateKey, dnsNames.Values())
	if err != nil {
		return err
	}
	if err := mongo.UpdateSSLKey(agentConfig.DataDir(), si.Cert, si.PrivateKey); err != nil {
		return err
	}
	return a.AgentConfigWriter.ChangeConfig(func(config agent.ConfigSetter) error {
		config.SetStateServingInfo(si)
		return nil
	})
}

// Run runs a machine agent.
func (a *MachineAgent) Run(*cmd.Context) error {

	defer a.tomb.Done()
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return fmt.Errorf("cannot read agent configuration: %v", err)
	}

	logger.Infof("machine agent %v start (%s [%s])", a.Tag(), jujuversion.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	// Before doing anything else, we need to make sure the certificate generated for
	// use by mongo to validate controller connections is correct. This needs to be done
	// before any possible restart of the mongo service.
	// See bug http://pad.lv/1434680
	if err := a.upgradeCertificateDNSNames(); err != nil {
		return errors.Annotate(err, "error upgrading server certificate")
	}

	if upgradeComplete, err := upgradesteps.NewLock(a); err != nil {
		return errors.Annotate(err, "error during creating upgrade completion channel")
	} else {
		a.upgradeComplete = upgradeComplete
	}

	agentConfig := a.CurrentConfig()
	createEngine := a.makeEngineCreator(agentConfig.UpgradedToVersion())
	charmrepo.CacheDir = filepath.Join(agentConfig.DataDir(), "charmcache")
	if err := a.createJujudSymlinks(agentConfig.DataDir()); err != nil {
		return err
	}
	a.runner.StartWorker("engine", createEngine)

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err := a.runner.Wait()
	switch errors.Cause(err) {
	case worker.ErrTerminateAgent:
		err = a.uninstallAgent()
	case worker.ErrRebootMachine:
		logger.Infof("Caught reboot error")
		err = a.executeRebootOrShutdown(params.ShouldReboot)
	case worker.ErrShutdownMachine:
		logger.Infof("Caught shutdown error")
		err = a.executeRebootOrShutdown(params.ShouldShutdown)
	}
	err = cmdutil.AgentDone(logger, err)
	a.tomb.Kill(err)
	return err
}

func (a *MachineAgent) makeEngineCreator(previousAgentVersion version.Number) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
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
		manifolds := machine.Manifolds(machine.ManifoldsConfig{
			PreviousAgentVersion: previousAgentVersion,
			Agent:                agent.APIHostPortsSetter{Agent: a},
			RootDir:              a.rootDir,
			AgentConfigChanged:   a.configChangedVal,
			UpgradeStepsLock:     a.upgradeComplete,
			UpgradeCheckLock:     a.initialUpgradeCheckComplete,
			OpenState:            a.initState,
			OpenStateForUpgrade:  a.openStateForUpgrade,
			StartStateWorkers:    a.startStateWorkers,
			StartAPIWorkers:      a.startAPIWorkers,
			PreUpgradeSteps:      upgrades.PreUpgradeSteps,
			LogSource:            a.bufferedLogs,
			NewDeployContext:     newDeployContext,
			Clock:                clock.WallClock,
		})
		if err := dependency.Install(engine, manifolds); err != nil {
			if err := worker.Stop(engine); err != nil {
				logger.Errorf("while stopping engine with bad manifolds: %v", err)
			}
			return nil, err
		}
		return engine, nil
	}
}

func (a *MachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	// At this stage, all API connections would have been closed
	// We need to reopen the API to clear the reboot flag after
	// scheduling the reboot. It may be cleaner to do this in the reboot
	// worker, before returning the ErrRebootMachine.
	conn, err := apicaller.OnlyConnect(a, apicaller.APIOpen)
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
	return worker.ErrRebootMachine
}

func (a *MachineAgent) ChangeConfig(mutate agent.ConfigMutator) error {
	err := a.AgentConfigWriter.ChangeConfig(mutate)
	a.configChangedVal.Set(true)
	return errors.Trace(err)
}

func (a *MachineAgent) maybeStopMongo(ver mongo.Version, isMaster bool) error {
	if !a.mongoInitialized {
		return nil
	}

	conf := a.AgentConfigWriter.CurrentConfig()
	v := conf.MongoVersion()

	logger.Errorf("Got version change %v", ver)
	// TODO(perrito666) replace with "read-only" mode for environment when
	// it is available.
	if ver.NewerThan(v) > 0 {
		err := a.AgentConfigWriter.ChangeConfig(func(config agent.ConfigSetter) error {
			config.SetMongoVersion(mongo.MongoUpgrade)
			return nil
		})
		if err != nil {
			return err
		}

	}
	return nil

}

// PrepareRestore will flag the agent to allow only a limited set
// of commands defined in
// "github.com/juju/juju/apiserver".allowedMethodsAboutToRestore
// the most noteworthy is:
// Backups.Restore: this will ensure that we can do all the file movements
// required for restore and no one will do changes while we do that.
// it will return error if the machine is already in this state.
func (a *MachineAgent) PrepareRestore() error {
	if a.restoreMode {
		return errors.Errorf("already in restore mode")
	}
	a.restoreMode = true
	return nil
}

// BeginRestore will flag the agent to disallow all commands since
// restore should be running and therefore making changes that
// would override anything done.
func (a *MachineAgent) BeginRestore() error {
	switch {
	case !a.restoreMode:
		return errors.Errorf("not in restore mode, cannot begin restoration")
	case a.restoring:
		return errors.Errorf("already restoring")
	}
	a.restoring = true
	return nil
}

// EndRestore will flag the agent to allow all commands
// This being invoked means that restore process failed
// since success restarts the agent.
func (a *MachineAgent) EndRestore() {
	a.restoreMode = false
	a.restoring = false
}

// newRestoreStateWatcherWorker will return a worker or err if there
// is a failure, the worker takes care of watching the state of
// restoreInfo doc and put the agent in the different restore modes.
func (a *MachineAgent) newRestoreStateWatcherWorker(st *state.State) (worker.Worker, error) {
	rWorker := func(stopch <-chan struct{}) error {
		return a.restoreStateWatcher(st, stopch)
	}
	return worker.NewSimpleWorker(rWorker), nil
}

// restoreChanged will be called whenever restoreInfo doc changes signaling a new
// step in the restore process.
func (a *MachineAgent) restoreChanged(st *state.State) error {
	status, err := st.RestoreInfo().Status()
	if err != nil {
		return errors.Annotate(err, "cannot read restore state")
	}
	switch status {
	case state.RestorePending:
		a.PrepareRestore()
	case state.RestoreInProgress:
		a.BeginRestore()
	case state.RestoreFailed:
		a.EndRestore()
	}
	return nil
}

// restoreStateWatcher watches for restoreInfo looking for changes in the restore process.
func (a *MachineAgent) restoreStateWatcher(st *state.State, stopch <-chan struct{}) error {
	restoreWatch := st.WatchRestoreInfoChanges()
	defer func() {
		restoreWatch.Kill()
		restoreWatch.Wait()
	}()

	for {
		select {
		case <-restoreWatch.Changes():
			if err := a.restoreChanged(st); err != nil {
				return err
			}
		case <-stopch:
			return nil
		}
	}
}

var newEnvirons = environs.New

// startAPIWorkers is called to start workers which rely on the
// machine agent's API connection (via the apiworkers manifold). It
// returns a Runner with a number of workers attached to it.
//
// The workers started here need to be converted to run under the
// dependency engine. Once they have all been converted, this method -
// and the apiworkers manifold - can be removed.
func (a *MachineAgent) startAPIWorkers(apiConn api.Connection) (_ worker.Worker, outErr error) {
	agentConfig := a.CurrentConfig()

	entity, err := apiagent.NewState(apiConn).Entity(a.Tag())
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

	runner := newConnRunner(apiConn)
	defer func() {
		// If startAPIWorkers exits early with an error, stop the
		// runner so that any already started runners aren't leaked.
		if outErr != nil {
			worker.Stop(runner)
		}
	}()

	modelConfig, err := apiagent.NewState(apiConn).ModelConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot read model config: %v", err)
	}

	// Perform the operations needed to set up hosting for containers.
	if err := a.setupContainerSupport(runner, apiConn, agentConfig); err != nil {
		cause := errors.Cause(err)
		if params.IsCodeDead(cause) || cause == worker.ErrTerminateAgent {
			return nil, worker.ErrTerminateAgent
		}
		return nil, fmt.Errorf("setting up container support: %v", err)
	}

	if isModelManager {

		// Published image metadata for some providers are in simple streams.
		// Providers that do not depend on simple streams do not need this worker.
		env, err := newEnvirons(modelConfig)
		if err != nil {
			return nil, errors.Annotate(err, "getting environ")
		}
		if _, ok := env.(simplestreams.HasRegion); ok {
			// Start worker that stores published image metadata in state.
			runner.StartWorker("imagemetadata", func() (worker.Worker, error) {
				return newMetadataUpdater(apiConn.MetadataUpdater()), nil
			})
		}

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

func (a *MachineAgent) setControllerNetworkConfig(apiConn api.Connection) error {
	machinerAPI := apimachiner.NewState(apiConn)
	agentConfig := a.CurrentConfig()

	tag := agentConfig.Tag().(names.MachineTag)
	machine, err := machinerAPI.Machine(tag)
	if errors.IsNotFound(err) || err == nil && machine.Life() == params.Dead {
		return worker.ErrTerminateAgent
	}
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", tag)
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
func (a *MachineAgent) openStateForUpgrade() (*state.State, error) {
	agentConfig := a.CurrentConfig()
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, errors.Trace(err)
	}
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, errors.New("no state info available")
	}
	st, err := state.Open(agentConfig.Model(), info, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// setupContainerSupport determines what containers can be run on this machine and
// initialises suitable infrastructure to support such containers.
func (a *MachineAgent) setupContainerSupport(runner worker.Runner, st api.Connection, agentConfig agent.Config) error {
	var supportedContainers []instance.ContainerType
	supportsContainers := container.ContainersSupported()
	if supportsContainers {
		supportedContainers = append(supportedContainers, instance.LXC, instance.LXD)
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
	runner worker.Runner,
	st api.Connection,
	containers []instance.ContainerType,
	agentConfig agent.Config,
) error {
	pr := st.Provisioner()
	tag := agentConfig.Tag().(names.MachineTag)
	machine, err := pr.Machine(tag)
	if errors.IsNotFound(err) || err == nil && machine.Life() == params.Dead {
		return worker.ErrTerminateAgent
	}
	if err != nil {
		return errors.Annotatef(err, "cannot load machine %s from state", tag)
	}
	if len(containers) == 0 {
		if err := machine.SupportsNoContainers(); err != nil {
			return errors.Annotatef(err, "clearing supported containers for %s", tag)
		}
		return nil
	}
	if err := machine.SetSupportedContainers(containers...); err != nil {
		return errors.Annotatef(err, "setting supported containers for %s", tag)
	}
	initLock, err := cmdutil.HookExecutionLock(agentConfig.DataDir())
	if err != nil {
		return err
	}
	// Start the watcher to fire when a container is first requested on the machine.
	modelUUID, err := st.ModelTag()
	if err != nil {
		return err
	}
	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	// There may not be a CA certificate private key available, and without
	// it we can't ensure that other Juju nodes can connect securely, so only
	// use an image URL getter if there's a private key.
	var imageURLGetter container.ImageURLGetter
	if agentConfig.Value(agent.AllowsSecureConnection) == "true" {
		cfg, err := pr.ModelConfig()
		if err != nil {
			return errors.Annotate(err, "unable to get environ config")
		}
		imageURLGetter = container.NewImageURLGetter(
			// Explicitly call the non-named constructor so if anyone
			// adds additional fields, this fails.
			container.ImageURLGetterConfig{
				ServerRoot:        st.Addr(),
				ModelUUID:         modelUUID.Id(),
				CACert:            []byte(agentConfig.CACert()),
				CloudimgBaseUrl:   cfg.CloudImageBaseURL(),
				Stream:            cfg.ImageStream(),
				ImageDownloadFunc: container.ImageDownloadURL,
			})
	}
	params := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: containers,
		ImageURLGetter:      imageURLGetter,
		Machine:             machine,
		Provisioner:         pr,
		Config:              agentConfig,
		InitLock:            initLock,
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

func (a *MachineAgent) initState(agentConfig agent.Config) (*state.State, error) {
	// Start MongoDB server and dial.
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}

	st, _, err := openState(agentConfig, stateWorkerDialOpts)
	if err != nil {
		return nil, err
	}

	reportOpenedState(st)

	return st, nil
}

// startStateWorkers returns a worker running all the workers that
// require a *state.State connection.
func (a *MachineAgent) startStateWorkers(st *state.State) (worker.Worker, error) {
	agentConfig := a.CurrentConfig()

	m, err := getMachine(st, agentConfig.Tag())
	if err != nil {
		return nil, errors.Annotate(err, "machine lookup")
	}

	runner := newConnRunner(st)
	singularRunner, err := newSingularStateRunner(runner, st, m)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, job := range m.Jobs() {
		switch job {
		case state.JobHostUnits:
			// Implemented elsewhere with workers that use the API.
		case state.JobManageNetworking:
			// Not used by state workers.
		case state.JobManageModel:
			useMultipleCPUs()
			a.startWorkerAfterUpgrade(runner, "model worker manager", func() (worker.Worker, error) {
				w, err := modelworkermanager.New(modelworkermanager.Config{
					Backend:    st,
					NewWorker:  a.startModelWorkers,
					ErrorDelay: worker.RestartDelay,
				})
				if err != nil {
					return nil, errors.Annotate(err, "cannot start model worker manager")
				}
				return w, nil
			})
			a.startWorkerAfterUpgrade(runner, "peergrouper", func() (worker.Worker, error) {
				w, err := peergrouperNew(st)
				if err != nil {
					return nil, errors.Annotate(err, "cannot start peergrouper worker")
				}
				return w, nil
			})
			a.startWorkerAfterUpgrade(runner, "restore", func() (worker.Worker, error) {
				w, err := a.newRestoreStateWatcherWorker(st)
				if err != nil {
					return nil, errors.Annotate(err, "cannot start backup-restorer worker")
				}
				return w, nil
			})
			a.startWorkerAfterUpgrade(runner, "mongoupgrade", func() (worker.Worker, error) {
				return newUpgradeMongoWorker(st, a.machineId, a.maybeStopMongo)
			})

			// certChangedChan is shared by multiple workers it's up
			// to the agent to close it rather than any one of the
			// workers.  It is possible that multiple cert changes
			// come in before the apiserver is up to receive them.
			// Specify a bigger buffer to prevent deadlock when
			// the apiserver isn't up yet.  Use a size of 10 since we
			// allow up to 7 controllers, and might also update the
			// addresses of the local machine (127.0.0.1, ::1, etc).
			//
			// TODO(cherylj/waigani) Remove this workaround when
			// certupdater and apiserver can properly manage dependencies
			// through the dependency engine.
			//
			// TODO(ericsnow) For now we simply do not close the channel.
			certChangedChan := make(chan params.StateServingInfo, 10)
			// Each time apiserver worker is restarted, we need a fresh copy of state due
			// to the fact that state holds lease managers which are killed and need to be reset.
			stateOpener := func() (*state.State, error) {
				logger.Debugf("opening state for apiserver worker")
				st, _, err := openState(agentConfig, stateWorkerDialOpts)
				return st, err
			}
			runner.StartWorker("apiserver", a.apiserverWorkerStarter(stateOpener, certChangedChan))
			var stateServingSetter certupdater.StateServingInfoSetter = func(info params.StateServingInfo, done <-chan struct{}) error {
				return a.ChangeConfig(func(config agent.ConfigSetter) error {
					config.SetStateServingInfo(info)
					logger.Infof("update apiserver worker with new certificate")
					select {
					case certChangedChan <- info:
						return nil
					case <-done:
						return nil
					}
				})
			}
			a.startWorkerAfterUpgrade(runner, "certupdater", func() (worker.Worker, error) {
				return newCertificateUpdater(m, agentConfig, st, st, stateServingSetter), nil
			})

			a.startWorkerAfterUpgrade(singularRunner, "dblogpruner", func() (worker.Worker, error) {
				return dblogpruner.New(st, dblogpruner.NewLogPruneParams()), nil
			})

			a.startWorkerAfterUpgrade(singularRunner, "txnpruner", func() (worker.Worker, error) {
				return txnpruner.New(st, time.Hour*2), nil
			})
		default:
			return nil, errors.Errorf("unknown job type %q", job)
		}
	}
	return runner, nil
}

// startModelWorkers starts the set of workers that run for every model
// in each controller.
func (a *MachineAgent) startModelWorkers(uuid string) (worker.Worker, error) {
	modelAgent, err := model.WrapAgent(a, uuid)
	if err != nil {
		return nil, errors.Trace(err)
	}

	engine, err := dependency.NewEngine(dependency.EngineConfig{
		IsFatal:     model.IsFatal,
		WorstError:  model.WorstError,
		Filter:      model.IgnoreErrRemoved,
		ErrorDelay:  3 * time.Second,
		BounceDelay: 10 * time.Millisecond,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifolds := modelManifolds(model.ManifoldsConfig{
		Agent:                       modelAgent,
		AgentConfigChanged:          a.configChangedVal,
		Clock:                       clock.WallClock,
		RunFlagDuration:             time.Minute,
		CharmRevisionUpdateInterval: 24 * time.Hour,
		InstPollerAggregationDelay:  3 * time.Second,
		// TODO(perrito666) the status history pruning numbers need
		// to be adjusting, after collecting user data from large install
		// bases, to numbers allowing a rich and useful back history.
		StatusHistoryPrunerMaxHistoryTime: 336 * time.Hour, // 2 weeks
		StatusHistoryPrunerMaxHistoryMB:   5120,            // 5G
		StatusHistoryPrunerInterval:       5 * time.Minute,
		SpacesImportedGate:                a.discoverSpacesComplete,
	})
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

func (a *MachineAgent) apiserverWorkerStarter(
	stateOpener func() (*state.State, error), certChanged chan params.StateServingInfo,
) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		st, err := stateOpener()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return a.newApiserverWorker(st, certChanged)
	}
}

func (a *MachineAgent) newApiserverWorker(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error) {
	agentConfig := a.CurrentConfig()
	// If the configuration does not have the required information,
	// it is currently not a recoverable error, so we kill the whole
	// agent, potentially enabling human intervention to fix
	// the agent's configuration file.
	info, ok := agentConfig.StateServingInfo()
	if !ok {
		return nil, &cmdutil.FatalError{"StateServingInfo not available and we need it"}
	}
	cert := []byte(info.Cert)
	key := []byte(info.PrivateKey)

	if len(cert) == 0 || len(key) == 0 {
		return nil, &cmdutil.FatalError{"configuration does not have controller cert/key"}
	}
	tag := agentConfig.Tag()
	dataDir := agentConfig.DataDir()
	logDir := agentConfig.LogDir()

	endpoint := net.JoinHostPort("", strconv.Itoa(info.APIPort))
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return nil, err
	}
	w, err := apiserver.NewServer(st, listener, apiserver.ServerConfig{
		Cert:        cert,
		Key:         key,
		Tag:         tag,
		DataDir:     dataDir,
		LogDir:      logDir,
		Validator:   a.limitLogins,
		CertChanged: certChanged,
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot start api server worker")
	}
	return w, nil
}

// limitLogins is called by the API server for each login attempt.
// it returns an error if upgrades or restore are running.
func (a *MachineAgent) limitLogins(req params.LoginRequest) error {
	if err := a.limitLoginsDuringRestore(req); err != nil {
		return err
	}
	if err := a.limitLoginsDuringUpgrade(req); err != nil {
		return err
	}
	return a.limitLoginsDuringMongoUpgrade(req)
}

func (a *MachineAgent) limitLoginsDuringMongoUpgrade(req params.LoginRequest) error {
	// If upgrade is running we will not be able to lock AgentConfigWriter
	// and it also means we are not upgrading mongo.
	if a.isUpgradeRunning() {
		return nil
	}
	cfg := a.AgentConfigWriter.CurrentConfig()
	ver := cfg.MongoVersion()
	if ver == mongo.MongoUpgrade {
		return errors.New("Upgrading Mongo")
	}
	return nil
}

// limitLoginsDuringRestore will only allow logins for restore related purposes
// while the different steps of restore are running.
func (a *MachineAgent) limitLoginsDuringRestore(req params.LoginRequest) error {
	var err error
	switch {
	case a.IsRestoreRunning():
		err = apiserver.RestoreInProgressError
	case a.IsRestorePreparing():
		err = apiserver.AboutToRestoreError
	}
	if err != nil {
		authTag, parseErr := names.ParseTag(req.AuthTag)
		if parseErr != nil {
			return errors.Annotate(err, "could not parse auth tag")
		}
		switch authTag := authTag.(type) {
		case names.UserTag:
			// use a restricted API mode
			return err
		case names.MachineTag:
			if authTag == a.Tag() {
				// allow logins from the local machine
				return nil
			}
		}
		return errors.Errorf("login for %q blocked because restore is in progress", authTag)
	}
	return nil
}

// limitLoginsDuringUpgrade is called by the API server for each login
// attempt. It returns an error if upgrades are in progress unless the
// login is for a user (i.e. a client) or the local machine.
func (a *MachineAgent) limitLoginsDuringUpgrade(req params.LoginRequest) error {
	if a.isUpgradeRunning() || a.isInitialUpgradeCheckPending() {
		authTag, err := names.ParseTag(req.AuthTag)
		if err != nil {
			return errors.Annotate(err, "could not parse auth tag")
		}
		switch authTag := authTag.(type) {
		case names.UserTag:
			// use a restricted API mode
			return params.UpgradeInProgressError
		case names.MachineTag:
			if authTag == a.Tag() {
				// allow logins from the local machine
				return nil
			}
		}
		return errors.Errorf("login for %q blocked because %s", authTag, params.CodeUpgradeInProgress)
	} else {
		return nil // allow all logins
	}
}

var stateWorkerServingConfigErr = errors.New("state worker started with no state serving info")

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

	mongoInstalled, err := mongo.IsServiceInstalled()
	if err != nil {
		return errors.Annotate(err, "error while checking if mongodb service is installed")
	}

	if !mongoInstalled {
		// EnsureMongoServer installs/upgrades the init config as necessary.
		ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
		if err != nil {
			return err
		}
		if err := cmdutil.EnsureMongoServer(ensureServerParams); err != nil {
			return err
		}
	}
	logger.Debugf("mongodb service is installed")

	// Mongo is installed, record the version.
	err = a.ChangeConfig(func(config agent.ConfigSetter) error {
		config.SetMongoVersion(mongo.InstalledVersion())
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "cannot set mongo version")
	}
	return nil
}

func openState(agentConfig agent.Config, dialOpts mongo.DialOpts) (_ *state.State, _ *state.Machine, err error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, nil, fmt.Errorf("no state info available")
	}
	st, err := state.Open(agentConfig.Model(), info, dialOpts, environs.NewStatePolicy())
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			st.Close()
		}
	}()
	m0, err := st.FindEntity(agentConfig.Tag())
	if err != nil {
		if errors.IsNotFound(err) {
			err = worker.ErrTerminateAgent
		}
		return nil, nil, err
	}
	m := m0.(*state.Machine)
	if m.Life() == state.Dead {
		return nil, nil, worker.ErrTerminateAgent
	}
	// Check the machine nonce as provisioned matches the agent.Conf value.
	if !m.CheckProvisioned(agentConfig.Nonce()) {
		// The agent is running on a different machine to the one it
		// should be according to state. It must stop immediately.
		logger.Errorf("running machine %v agent on inappropriate instance", m)
		return nil, nil, worker.ErrTerminateAgent
	}
	return st, m, nil
}

func getMachine(st *state.State, tag names.Tag) (*state.Machine, error) {
	m0, err := st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return m0.(*state.Machine), nil
}

// startWorkerAfterUpgrade starts a worker to run the specified child worker
// but only after waiting for upgrades to complete.
func (a *MachineAgent) startWorkerAfterUpgrade(runner worker.Runner, name string, start func() (worker.Worker, error)) {
	runner.StartWorker(name, func() (worker.Worker, error) {
		return a.upgradeWaiterWorker(name, start), nil
	})
}

// upgradeWaiterWorker runs the specified worker after upgrades have completed.
func (a *MachineAgent) upgradeWaiterWorker(name string, start func() (worker.Worker, error)) worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
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
		worker, err := start()
		if err != nil {
			return err
		}
		// Wait for worker to finish or for us to be stopped.
		waitCh := make(chan error)
		go func() {
			waitCh <- worker.Wait()
		}()
		select {
		case err := <-waitCh:
			logger.Debugf("worker %q exited with %v", name, err)
			return err
		case <-stop:
			logger.Debugf("stopping so killing worker %q", name)
			worker.Kill()
		}
		return <-waitCh // Ensure worker has stopped before returning.
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
	for _, link := range []string{jujuRun, jujuDumpLogs} {
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
	for _, link := range []string{jujuRun, jujuDumpLogs} {
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
			errs = append(errs, fmt.Errorf("cannot remove service %q: %v", agentServiceName, err))
		} else if err := svc.Remove(); err != nil {
			errs = append(errs, fmt.Errorf("cannot remove service %q: %v", agentServiceName, err))
		}
	}

	errs = append(errs, a.removeJujudSymlinks()...)

	// TODO(fwereade): surely this shouldn't be happening here? Once we're
	// at this point we should expect to be killed in short order; if this
	// work is remotely important we should be blocking machine death on
	// its completion.
	insideContainer := container.RunningInContainer()
	if insideContainer {
		// We're running inside LXC, so loop devices may leak. Detach
		// any loop devices that are backed by files on this machine.
		//
		// It is necessary to do this here as well as in container/lxc,
		// as container/lxc needs to check in the container's rootfs
		// to see if the loop device is attached to the container; that
		// will fail if the data-dir is removed first.
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
	return fmt.Errorf("uninstall failed: %v", errs)
}

func newConnRunner(conns ...cmdutil.Pinger) worker.Runner {
	return worker.NewRunner(cmdutil.ConnectionIsFatal(logger, conns...), cmdutil.MoreImportant, worker.RestartDelay)
}

type MongoSessioner interface {
	MongoSession() *mgo.Session
}

func newSingularStateRunner(runner worker.Runner, st MongoSessioner, m *state.Machine) (worker.Runner, error) {
	singularStateConn := singularStateConn{st.MongoSession(), m}
	singularRunner, err := newSingularRunner(runner, singularStateConn)
	if err != nil {
		return nil, errors.Annotate(err, "cannot make singular State Runner")
	}
	return singularRunner, err
}

// singularStateConn implements singular.Conn on
// top of a State connection.
type singularStateConn struct {
	session *mgo.Session
	machine *state.Machine
}

func (c singularStateConn) IsMaster() (bool, error) {
	return mongo.IsMaster(c.session, c.machine)
}

func (c singularStateConn) Ping() error {
	return c.session.Ping()
}

func metricAPI(st api.Connection) (metricsmanager.MetricsManagerClient, error) {
	client, err := metricsmanager.NewClient(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, agentConfig agent.Config) deployer.Context {
	return deployer.NewSimpleContext(agentConfig, st)
}
