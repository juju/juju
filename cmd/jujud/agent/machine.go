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
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
	"github.com/juju/utils/voyeur"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	apideployer "github.com/juju/juju/api/deployer"
	"github.com/juju/juju/api/metricsmanager"
	apiupgrader "github.com/juju/juju/api/upgrader"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cmd/jujud/reboot"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/cmd/jujud/util/password"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/container/lxc/lxcutils"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statestorage "github.com/juju/juju/state/storage"
	"github.com/juju/juju/storage/looputil"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/charmrevisionworker"
	"github.com/juju/juju/worker/cleaner"
	"github.com/juju/juju/worker/conv2state"
	"github.com/juju/juju/worker/dblogpruner"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/envworkermanager"
	"github.com/juju/juju/worker/firewaller"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/localstorage"
	workerlogger "github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/metricworker"
	"github.com/juju/juju/worker/minunitsworker"
	"github.com/juju/juju/worker/networker"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/proxyupdater"
	rebootworker "github.com/juju/juju/worker/reboot"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/statushistorypruner"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/txnpruner"
	"github.com/juju/juju/worker/upgrader"
)

const bootstrapMachineId = "0"

var (
	logger     = loggo.GetLogger("juju.cmd.jujud")
	retryDelay = 3 * time.Second
	JujuRun    = paths.MustSucceed(paths.JujuRun(version.Current.Series))

	// The following are defined as variables to allow the tests to
	// intercept calls to the functions.
	useMultipleCPUs          = utils.UseMultipleCPUs
	maybeInitiateMongoServer = peergrouper.MaybeInitiateMongoServer
	ensureMongoAdminUser     = mongo.EnsureAdminUser
	newSingularRunner        = singular.New
	peergrouperNew           = peergrouper.New
	newMachiner              = machiner.NewMachiner
	newNetworker             = networker.NewNetworker
	newFirewaller            = firewaller.NewFirewaller
	newDiskManager           = diskmanager.NewWorker
	newStorageWorker         = storageprovisioner.NewStorageProvisioner
	newCertificateUpdater    = certupdater.NewCertificateUpdater
	newResumer               = resumer.NewResumer
	newInstancePoller        = instancepoller.NewWorker
	newCleaner               = cleaner.NewCleaner
	newAddresser             = addresser.NewWorker
	reportOpenedState        = func(io.Closer) {}
	reportOpenedAPI          = func(io.Closer) {}
	getMetricAPI             = metricAPI
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
	// environments that have been upgraded, we need to explicitly remove the
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
	agentConfig := a.currentConfig.CurrentConfig()

	// the context's stderr is set as the loggo writer in github.com/juju/cmd/logging.go
	a.ctx.Stderr = &lumberjack.Logger{
		Filename:   agent.LogFilename(agentConfig),
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
	loopDeviceManager looputil.LoopDeviceManager,
) func(string) *MachineAgent {
	return func(machineId string) *MachineAgent {
		return NewMachineAgent(
			machineId,
			agentConfWriter,
			bufferedLogs,
			NewUpgradeWorkerContext(),
			worker.NewRunner(cmdutil.IsFatal, cmdutil.MoreImportant),
			loopDeviceManager,
		)
	}
}

// NewMachineAgent instantiates a new MachineAgent.
func NewMachineAgent(
	machineId string,
	agentConfWriter AgentConfigWriter,
	bufferedLogs logsender.LogRecordCh,
	upgradeWorkerContext *upgradeWorkerContext,
	runner worker.Runner,
	loopDeviceManager looputil.LoopDeviceManager,
) *MachineAgent {
	return &MachineAgent{
		machineId:            machineId,
		AgentConfigWriter:    agentConfWriter,
		bufferedLogs:         bufferedLogs,
		upgradeWorkerContext: upgradeWorkerContext,
		workersStarted:       make(chan struct{}),
		runner:               runner,
		initialAgentUpgradeCheckComplete: make(chan struct{}),
		loopDeviceManager:                loopDeviceManager,
	}
}

// MachineAgent is responsible for tying together all functionality
// needed to orchestrate a Jujud instance which controls a machine.
type MachineAgent struct {
	AgentConfigWriter

	tomb                 tomb.Tomb
	machineId            string
	previousAgentVersion version.Number
	runner               worker.Runner
	bufferedLogs         logsender.LogRecordCh
	configChangedVal     voyeur.Value
	upgradeWorkerContext *upgradeWorkerContext
	workersStarted       chan struct{}

	// XXX(fwereade): these smell strongly of goroutine-unsafeness.
	restoreMode bool
	restoring   bool

	// Used to signal that the upgrade worker will not
	// reboot the agent on startup because there are no
	// longer any immediately pending agent upgrades.
	// Channel used as a selectable bool (closed means true).
	initialAgentUpgradeCheckComplete chan struct{}

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

func (a *MachineAgent) isAgentUpgradePending() bool {
	select {
	case <-a.initialAgentUpgradeCheckComplete:
		return false
	default:
		return true
	}
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

// Dying returns the channel that can be used to see if the machine
// agent is terminating.
func (a *MachineAgent) Dying() <-chan struct{} {
	return a.tomb.Dying()
}

// upgradeCertificateDNSNames ensure that the state server certificate
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

	logger.Infof("machine agent %v start (%s [%s])", a.Tag(), version.Current, runtime.Compiler)
	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}

	// Before doing anything else, we need to make sure the certificate generated for
	// use by mongo to validate state server connections is correct. This needs to be done
	// before any possible restart of the mongo service.
	// See bug http://pad.lv/1434680
	if err := a.upgradeCertificateDNSNames(); err != nil {
		return errors.Annotate(err, "error upgrading server certificate")
	}

	agentConfig := a.CurrentConfig()

	if err := a.upgradeWorkerContext.InitializeUsingAgent(a); err != nil {
		return errors.Annotate(err, "error during upgradeWorkerContext initialisation")
	}
	a.configChangedVal.Set(struct{}{})
	a.previousAgentVersion = agentConfig.UpgradedToVersion()

	network.InitializeFromConfig(agentConfig)
	charmrepo.CacheDir = filepath.Join(agentConfig.DataDir(), "charmcache")
	if err := a.createJujuRun(agentConfig.DataDir()); err != nil {
		return fmt.Errorf("cannot create juju run symlink: %v", err)
	}
	a.runner.StartWorker("api", a.APIWorker)
	a.runner.StartWorker("statestarter", a.newStateStarterWorker)
	a.runner.StartWorker("termination", func() (worker.Worker, error) {
		return terminationworker.NewWorker(), nil
	})

	// At this point, all workers will have been configured to start
	close(a.workersStarted)
	err := a.runner.Wait()
	switch err {
	case worker.ErrTerminateAgent:
		err = a.uninstallAgent(agentConfig)
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

func (a *MachineAgent) executeRebootOrShutdown(action params.RebootAction) error {
	agentCfg := a.CurrentConfig()
	// At this stage, all API connections would have been closed
	// We need to reopen the API to clear the reboot flag after
	// scheduling the reboot. It may be cleaner to do this in the reboot
	// worker, before returning the ErrRebootMachine.
	st, _, err := apicaller.OpenAPIState(a)
	if err != nil {
		logger.Infof("Reboot: Error connecting to state")
		return errors.Trace(err)
	}
	// block until all units/containers are ready, and reboot/shutdown
	finalize, err := reboot.NewRebootWaiter(st, agentCfg)
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
	a.configChangedVal.Set(struct{}{})
	if err != nil {
		return errors.Trace(err)
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
	rinfo, err := st.RestoreInfoSetter()
	if err != nil {
		return errors.Annotate(err, "cannot read restore state")
	}
	switch rinfo.Status() {
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

// newStateStarterWorker wraps stateStarter in a simple worker for use in
// a.runner.StartWorker.
func (a *MachineAgent) newStateStarterWorker() (worker.Worker, error) {
	return worker.NewSimpleWorker(a.stateStarter), nil
}

// stateStarter watches for changes to the agent configuration, and
// starts or stops the state worker as appropriate. We watch the agent
// configuration because the agent configuration has all the details
// that we need to start a state server, whether they have been cached
// or read from the state.
//
// It will stop working as soon as stopch is closed.
func (a *MachineAgent) stateStarter(stopch <-chan struct{}) error {
	confWatch := a.configChangedVal.Watch()
	defer confWatch.Close()
	watchCh := make(chan struct{})
	go func() {
		for confWatch.Next() {
			watchCh <- struct{}{}
		}
	}()
	for {
		select {
		case <-watchCh:
			agentConfig := a.CurrentConfig()

			// N.B. StartWorker and StopWorker are idempotent.
			_, ok := agentConfig.StateServingInfo()
			if ok {
				a.runner.StartWorker("state", func() (worker.Worker, error) {
					return a.StateWorker()
				})
			} else {
				a.runner.StopWorker("state")
			}
		case <-stopch:
			return nil
		}
	}
}

// APIWorker returns a Worker that connects to the API and starts any
// workers that need an API connection.
func (a *MachineAgent) APIWorker() (_ worker.Worker, err error) {
	st, entity, err := apicaller.OpenAPIState(a)
	if err != nil {
		return nil, err
	}
	reportOpenedAPI(st)

	defer func() {
		// TODO(fwereade): this is not properly tested. Old tests were evil
		// (dependent on injecting an error in a patched-out upgrader API
		// that shouldn't even be used at this level)... so I just deleted
		// them. Not a major worry: this whole method will become redundant
		// when we switch to the dependency engine (and specifically use
		// worker/apicaller to connect).
		if err != nil {
			if err := st.Close(); err != nil {
				logger.Errorf("while closing API: %v", err)
			}
		}
	}()

	agentConfig := a.CurrentConfig()
	for _, job := range entity.Jobs() {
		if job.NeedsState() {
			info, err := st.Agent().StateServingInfo()
			if err != nil {
				return nil, fmt.Errorf("cannot get state serving info: %v", err)
			}
			err = a.ChangeConfig(func(config agent.ConfigSetter) error {
				config.SetStateServingInfo(info)
				return nil
			})
			if err != nil {
				return nil, err
			}
			agentConfig = a.CurrentConfig()
			break
		}
	}

	runner := newConnRunner(st)

	// Run the agent upgrader and the upgrade-steps worker without waiting for
	// the upgrade steps to complete.
	runner.StartWorker("upgrader", a.agentUpgraderWorkerStarter(st.Upgrader(), agentConfig))
	runner.StartWorker("upgrade-steps", a.upgradeStepsWorkerStarter(st, entity.Jobs()))

	// All other workers must wait for the upgrade steps to complete before starting.
	a.startWorkerAfterUpgrade(runner, "api-post-upgrade", func() (worker.Worker, error) {
		return a.postUpgradeAPIWorker(st, agentConfig, entity)
	})
	return cmdutil.NewCloseWorker(logger, runner, st), nil // Note: a worker.Runner is itself a worker.Worker.
}

func (a *MachineAgent) postUpgradeAPIWorker(
	st api.Connection,
	agentConfig agent.Config,
	entity *apiagent.Entity,
) (worker.Worker, error) {

	var isEnvironManager bool
	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageEnviron {
			isEnvironManager = true
			break
		}
	}

	runner := newConnRunner(st)

	// TODO(fwereade): this is *still* a hideous layering violation, but at least
	// it's confined to jujud rather than extending into the worker itself.
	// Start this worker first to try and get proxy settings in place
	// before we do anything else.
	writeSystemFiles := shouldWriteProxyFiles(agentConfig)
	runner.StartWorker("proxyupdater", func() (worker.Worker, error) {
		return proxyupdater.New(st.Environment(), writeSystemFiles), nil
	})

	if isEnvironManager {
		runner.StartWorker("resumer", func() (worker.Worker, error) {
			// The action of resumer is so subtle that it is not tested,
			// because we can't figure out how to do so without
			// brutalising the transaction log.
			return newResumer(st.Resumer()), nil
		})
	}

	if feature.IsDbLogEnabled() {
		runner.StartWorker("logsender", func() (worker.Worker, error) {
			return logsender.New(a.bufferedLogs, gate.AlreadyUnlocked{}, a), nil
		})
	}

	envConfig, err := st.Environment().EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot read environment config: %v", err)
	}
	ignoreMachineAddresses, _ := envConfig.IgnoreMachineAddresses()
	if ignoreMachineAddresses {
		logger.Infof("machine addresses not used, only addresses from provider")
	}
	runner.StartWorker("machiner", func() (worker.Worker, error) {
		accessor := machiner.APIMachineAccessor{st.Machiner()}
		return newMachiner(accessor, agentConfig, ignoreMachineAddresses), nil
	})
	runner.StartWorker("reboot", func() (worker.Worker, error) {
		reboot, err := st.Reboot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		lock, err := cmdutil.HookExecutionLock(cmdutil.DataDir)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return rebootworker.NewReboot(reboot, agentConfig, lock)
	})
	runner.StartWorker("apiaddressupdater", func() (worker.Worker, error) {
		addressUpdater := agent.APIHostPortsSetter{a}
		return apiaddressupdater.NewAPIAddressUpdater(st.Machiner(), addressUpdater), nil
	})
	runner.StartWorker("logger", func() (worker.Worker, error) {
		return workerlogger.NewLogger(st.Logger(), agentConfig), nil
	})

	if !featureflag.Enabled(feature.DisableRsyslog) {
		rsyslogMode := rsyslog.RsyslogModeForwarding
		if isEnvironManager {
			rsyslogMode = rsyslog.RsyslogModeAccumulate
		}

		runner.StartWorker("rsyslog", func() (worker.Worker, error) {
			return cmdutil.NewRsyslogConfigWorker(st.Rsyslog(), agentConfig, rsyslogMode)
		})
	}

	if !isEnvironManager {
		runner.StartWorker("stateconverter", func() (worker.Worker, error) {
			return worker.NewNotifyWorker(conv2state.New(st.Machiner(), a)), nil
		})
	}

	runner.StartWorker("diskmanager", func() (worker.Worker, error) {
		api, err := st.DiskManager()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newDiskManager(diskmanager.DefaultListBlockDevices, api), nil
	})
	runner.StartWorker("storageprovisioner-machine", func() (worker.Worker, error) {
		scope := agentConfig.Tag()
		api := st.StorageProvisioner(scope)
		storageDir := filepath.Join(agentConfig.DataDir(), "storage")
		return newStorageWorker(
			scope, storageDir, api, api, api, api, api, api,
			clock.WallClock,
		), nil
	})

	// Check if the network management is disabled.
	disableNetworkManagement, _ := envConfig.DisableNetworkManagement()
	if disableNetworkManagement {
		logger.Infof("network management is disabled")
	}

	// Start networker depending on configuration and job.
	intrusiveMode := false
	for _, job := range entity.Jobs() {
		if job == multiwatcher.JobManageNetworking {
			intrusiveMode = true
			break
		}
	}
	intrusiveMode = intrusiveMode && !disableNetworkManagement
	runner.StartWorker("networker", func() (worker.Worker, error) {
		return newNetworker(st.Networker(), agentConfig, intrusiveMode, networker.DefaultConfigBaseDir)
	})

	// If not a local provider bootstrap machine, start the worker to
	// manage SSH keys.
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType != provider.Local || a.machineId != bootstrapMachineId {
		runner.StartWorker("authenticationworker", func() (worker.Worker, error) {
			return authenticationworker.NewWorker(st.KeyUpdater(), agentConfig), nil
		})
	}

	// Perform the operations needed to set up hosting for containers.
	if err := a.setupContainerSupport(runner, st, entity, agentConfig); err != nil {
		cause := errors.Cause(err)
		if params.IsCodeDead(cause) || cause == worker.ErrTerminateAgent {
			return nil, worker.ErrTerminateAgent
		}
		return nil, fmt.Errorf("setting up container support: %v", err)
	}
	for _, job := range entity.Jobs() {
		switch job {
		case multiwatcher.JobHostUnits:
			runner.StartWorker("deployer", func() (worker.Worker, error) {
				apiDeployer := st.Deployer()
				context := newDeployContext(apiDeployer, agentConfig)
				return deployer.NewDeployer(apiDeployer, context), nil
			})
		case multiwatcher.JobManageEnviron:
			runner.StartWorker("identity-file-writer", func() (worker.Worker, error) {
				inner := func(<-chan struct{}) error {
					agentConfig := a.CurrentConfig()
					return agent.WriteSystemIdentityFile(agentConfig)
				}
				return worker.NewSimpleWorker(inner), nil
			})
		case multiwatcher.JobManageStateDeprecated:
			// Legacy environments may set this, but we ignore it.
		default:
			// TODO(dimitern): Once all workers moved over to using
			// the API, report "unknown job type" here.
		}
	}

	return cmdutil.NewCloseWorker(logger, runner, st), nil // Note: a worker.Runner is itself a worker.Worker.
}

// Restart restarts the agent's service.
func (a *MachineAgent) Restart() error {
	name := a.CurrentConfig().Value(agent.AgentServiceName)
	return service.Restart(name)
}

func (a *MachineAgent) upgradeStepsWorkerStarter(
	st api.Connection,
	jobs []multiwatcher.MachineJob,
) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		return a.upgradeWorkerContext.Worker(a, st, jobs), nil
	}
}

func (a *MachineAgent) agentUpgraderWorkerStarter(
	st *apiupgrader.State,
	agentConfig agent.Config,
) func() (worker.Worker, error) {
	return func() (worker.Worker, error) {
		return upgrader.NewAgentUpgrader(
			st,
			agentConfig,
			a.previousAgentVersion,
			a.upgradeWorkerContext.IsUpgradeRunning,
			a.initialAgentUpgradeCheckComplete,
		), nil
	}
}

// shouldWriteProxyFiles returns true, unless the supplied conf identifies the
// machine agent running directly on the host system in a local environment.
var shouldWriteProxyFiles = func(conf agent.Config) bool {
	if conf.Value(agent.ProviderType) != provider.Local {
		return true
	}
	return conf.Tag() != names.NewMachineTag(bootstrapMachineId)
}

// setupContainerSupport determines what containers can be run on this machine and
// initialises suitable infrastructure to support such containers.
func (a *MachineAgent) setupContainerSupport(runner worker.Runner, st api.Connection, entity *apiagent.Entity, agentConfig agent.Config) error {
	var supportedContainers []instance.ContainerType
	// LXC containers are only supported on bare metal and fully virtualized linux systems
	// Nested LXC containers and Windows machines cannot run LXC containers
	supportsLXC, err := lxc.IsLXCSupported()
	if err != nil {
		logger.Warningf("no lxc containers possible: %v", err)
	}
	if err == nil && supportsLXC {
		supportedContainers = append(supportedContainers, instance.LXC)
	}

	supportsKvm, err := kvm.IsKVMSupported()
	if err != nil {
		logger.Warningf("determining kvm support: %v\nno kvm containers possible", err)
	}
	if err == nil && supportsKvm {
		supportedContainers = append(supportedContainers, instance.KVM)
	}
	return a.updateSupportedContainers(runner, st, entity.Tag(), supportedContainers, agentConfig)
}

// updateSupportedContainers records in state that a machine can run the specified containers.
// It starts a watcher and when a container of a given type is first added to the machine,
// the watcher is killed, the machine is set up to be able to start containers of the given type,
// and a suitable provisioner is started.
func (a *MachineAgent) updateSupportedContainers(
	runner worker.Runner,
	st api.Connection,
	machineTag string,
	containers []instance.ContainerType,
	agentConfig agent.Config,
) error {
	pr := st.Provisioner()
	tag, err := names.ParseMachineTag(machineTag)
	if err != nil {
		return err
	}
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
	envUUID, err := st.EnvironTag()
	if err != nil {
		return err
	}
	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	// There may not be a CA certificate private key available, and without
	// it we can't ensure that other Juju nodes can connect securely, so only
	// use an image URL getter if there's a private key.
	var imageURLGetter container.ImageURLGetter
	if agentConfig.Value(agent.AllowsSecureConnection) == "true" {
		imageURLGetter = container.NewImageURLGetter(st.Addr(), envUUID.Id(), []byte(agentConfig.CACert()))
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
		return worker.NewStringsWorker(handler), nil
	})
	return nil
}

// StateWorker returns a worker running all the workers that require
// a *state.State connection.
func (a *MachineAgent) StateWorker() (worker.Worker, error) {
	agentConfig := a.CurrentConfig()

	// Start MongoDB server and dial.
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}
	st, m, err := openState(agentConfig, stateWorkerDialOpts)
	if err != nil {
		return nil, err
	}
	reportOpenedState(st)

	stor := statestorage.NewStorage(st.EnvironUUID(), st.MongoSession())
	registerSimplestreamsDataSource(stor)

	runner := newConnRunner(st)
	singularRunner, err := newSingularStateRunner(runner, st, m)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Take advantage of special knowledge here in that we will only ever want
	// the storage provider on one machine, and that is the "bootstrap" node.
	providerType := agentConfig.Value(agent.ProviderType)
	if (providerType == provider.Local || provider.IsManual(providerType)) && m.Id() == bootstrapMachineId {
		a.startWorkerAfterUpgrade(runner, "local-storage", func() (worker.Worker, error) {
			// TODO(axw) 2013-09-24 bug #1229507
			// Make another job to enable storage.
			// There's nothing special about this.
			return localstorage.NewWorker(agentConfig), nil
		})
	}
	for _, job := range m.Jobs() {
		switch job {
		case state.JobHostUnits:
			// Implemented in APIWorker.
		case state.JobManageEnviron:
			useMultipleCPUs()
			a.startWorkerAfterUpgrade(runner, "env worker manager", func() (worker.Worker, error) {
				return envworkermanager.NewEnvWorkerManager(st, a.startEnvWorkers), nil
			})
			a.startWorkerAfterUpgrade(runner, "peergrouper", func() (worker.Worker, error) {
				return peergrouperNew(st)
			})
			a.startWorkerAfterUpgrade(runner, "restore", func() (worker.Worker, error) {
				return a.newRestoreStateWatcherWorker(st)
			})
			certChangedChan := make(chan params.StateServingInfo, 1)
			runner.StartWorker("apiserver", a.apiserverWorkerStarter(st, certChangedChan))
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
				return newCertificateUpdater(m, agentConfig, st, st, stateServingSetter, certChangedChan), nil
			})

			if feature.IsDbLogEnabled() {
				a.startWorkerAfterUpgrade(singularRunner, "dblogpruner", func() (worker.Worker, error) {
					return dblogpruner.New(st, dblogpruner.NewLogPruneParams()), nil
				})
			}
			a.startWorkerAfterUpgrade(singularRunner, "statushistorypruner", func() (worker.Worker, error) {
				return statushistorypruner.New(st, statushistorypruner.NewHistoryPrunerParams()), nil
			})

			a.startWorkerAfterUpgrade(singularRunner, "txnpruner", func() (worker.Worker, error) {
				return txnpruner.New(st, time.Hour*2), nil
			})

		case state.JobManageStateDeprecated:
			// Legacy environments may set this, but we ignore it.
		default:
			logger.Warningf("ignoring unknown job %q", job)
		}
	}
	return cmdutil.NewCloseWorker(logger, runner, st), nil
}

// startEnvWorkers starts state server workers that need to run per
// environment.
func (a *MachineAgent) startEnvWorkers(
	ssSt envworkermanager.InitialState,
	st *state.State,
) (_ worker.Worker, err error) {
	envUUID := st.EnvironUUID()
	defer errors.DeferredAnnotatef(&err, "failed to start workers for env %s", envUUID)
	logger.Infof("starting workers for env %s", envUUID)

	// Establish API connection for this environment.
	agentConfig := a.CurrentConfig()
	apiInfo := agentConfig.APIInfo()
	apiInfo.EnvironTag = st.EnvironTag()
	apiSt, err := apicaller.OpenAPIStateUsingInfo(apiInfo, agentConfig.OldPassword())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create a runner for workers specific to this
	// environment. Either the State or API connection failing will be
	// considered fatal, killing the runner and all its workers.
	runner := newConnRunner(st, apiSt)
	defer func() {
		if err != nil && runner != nil {
			runner.Kill()
			runner.Wait()
		}
	}()
	// Close the API connection when the runner for this environment dies.
	go func() {
		runner.Wait()
		err := apiSt.Close()
		if err != nil {
			logger.Errorf("failed to close API connection for env %s: %v", envUUID, err)
		}
	}()

	// Create a singular runner for this environment.
	machine, err := ssSt.Machine(a.machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	singularRunner, err := newSingularStateRunner(runner, ssSt, machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil && singularRunner != nil {
			singularRunner.Kill()
			singularRunner.Wait()
		}
	}()

	// Start workers that depend on a *state.State.
	// TODO(fwereade): 2015-04-21 THIS SHALL NOT PASS
	// Seriously, these should all be using the API.
	singularRunner.StartWorker("minunitsworker", func() (worker.Worker, error) {
		return minunitsworker.NewMinUnitsWorker(st), nil
	})

	// Start workers that use an API connection.
	singularRunner.StartWorker("environ-provisioner", func() (worker.Worker, error) {
		return provisioner.NewEnvironProvisioner(apiSt.Provisioner(), agentConfig), nil
	})
	singularRunner.StartWorker("environ-storageprovisioner", func() (worker.Worker, error) {
		scope := st.EnvironTag()
		api := apiSt.StorageProvisioner(scope)
		return newStorageWorker(
			scope, "", api, api, api, api, api, api,
			clock.WallClock,
		), nil
	})
	singularRunner.StartWorker("charm-revision-updater", func() (worker.Worker, error) {
		return charmrevisionworker.NewRevisionUpdateWorker(apiSt.CharmRevisionUpdater()), nil
	})
	runner.StartWorker("metricmanagerworker", func() (worker.Worker, error) {
		return metricworker.NewMetricsManager(getMetricAPI(apiSt))
	})
	singularRunner.StartWorker("instancepoller", func() (worker.Worker, error) {
		return newInstancePoller(apiSt.InstancePoller()), nil
	})
	singularRunner.StartWorker("cleaner", func() (worker.Worker, error) {
		return newCleaner(apiSt.Cleaner()), nil
	})

	// Addresser Worker needs a networking environment. Check
	// first before starting the worker.
	isNetEnv, err := isNetworkingEnvironment(apiSt)
	if err != nil {
		return nil, errors.Annotate(err, "checking environment networking support")
	}
	if isNetEnv {
		singularRunner.StartWorker("addresserworker", func() (worker.Worker, error) {
			return newAddresser(apiSt.Addresser())
		})
	} else {
		logger.Debugf("address deallocation not supported: not starting addresser worker")
	}
	// TODO(axw) 2013-09-24 bug #1229506
	// Make another job to enable the firewaller. Not all
	// environments are capable of managing ports
	// centrally.
	fwMode, err := getFirewallMode(apiSt)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get firewall mode")
	}
	if fwMode != config.FwNone {
		singularRunner.StartWorker("firewaller", func() (worker.Worker, error) {
			return newFirewaller(apiSt.Firewaller())
		})
	} else {
		logger.Debugf("not starting firewaller worker - firewall-mode is %q", fwMode)
	}

	return runner, nil
}

func isNetworkingEnvironment(apiConn api.Connection) (bool, error) {
	envConfig, err := apiConn.Environment().EnvironConfig()
	if err != nil {
		return false, errors.Annotate(err, "cannot read environment config")
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return false, errors.Annotate(err, "cannot get environment")
	}
	_, ok := environs.SupportsNetworking(env)
	return ok, nil
}

var getFirewallMode = _getFirewallMode

func _getFirewallMode(apiSt api.Connection) (string, error) {
	envConfig, err := apiSt.Environment().EnvironConfig()
	if err != nil {
		return "", errors.Annotate(err, "cannot read environment config")
	}
	return envConfig.FirewallMode(), nil
}

// stateWorkerDialOpts is a mongo.DialOpts suitable
// for use by StateWorker to dial mongo.
//
// This must be overridden in tests, as it assumes
// journaling is enabled.
var stateWorkerDialOpts mongo.DialOpts

func (a *MachineAgent) apiserverWorkerStarter(st *state.State, certChanged chan params.StateServingInfo) func() (worker.Worker, error) {
	return func() (worker.Worker, error) { return a.newApiserverWorker(st, certChanged) }
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
		return nil, &cmdutil.FatalError{"configuration does not have state server cert/key"}
	}
	tag := agentConfig.Tag()
	dataDir := agentConfig.DataDir()
	logDir := agentConfig.LogDir()

	endpoint := net.JoinHostPort("", strconv.Itoa(info.APIPort))
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return nil, err
	}
	return apiserver.NewServer(st, listener, apiserver.ServerConfig{
		Cert:        cert,
		Key:         key,
		Tag:         tag,
		DataDir:     dataDir,
		LogDir:      logDir,
		Validator:   a.limitLogins,
		CertChanged: certChanged,
	})
}

// limitLogins is called by the API server for each login attempt.
// it returns an error if upgrades or restore are running.
func (a *MachineAgent) limitLogins(req params.LoginRequest) error {
	if err := a.limitLoginsDuringRestore(req); err != nil {
		return err
	}
	return a.limitLoginsDuringUpgrade(req)
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
	if a.upgradeWorkerContext.IsUpgradeRunning() || a.isAgentUpgradePending() {
		authTag, err := names.ParseTag(req.AuthTag)
		if err != nil {
			return errors.Annotate(err, "could not parse auth tag")
		}
		switch authTag := authTag.(type) {
		case names.UserTag:
			// use a restricted API mode
			return apiserver.UpgradeInProgressError
		case names.MachineTag:
			if authTag == a.Tag() {
				// allow logins from the local machine
				return nil
			}
		}
		return errors.Errorf("login for %q blocked because %s", authTag, apiserver.UpgradeInProgressError.Error())
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

	// Many of the steps here, such as adding the state server to the
	// admin DB and initiating the replicaset, are once-only actions,
	// required when upgrading from a pre-HA-capable
	// environment. These calls won't do anything if the thing they
	// need to set up has already been done.
	var needReplicasetInit = false
	var machineAddrs []network.Address

	mongoInstalled, err := mongo.IsServiceInstalled(agentConfig.Value(agent.Namespace))
	if err != nil {
		return errors.Annotate(err, "error while checking if mongodb service is installed")
	}

	if mongoInstalled {
		logger.Debugf("mongodb service is installed")

		if _, err := a.ensureMongoAdminUser(agentConfig); err != nil {
			return errors.Trace(err)
		}

		if err := a.ensureMongoSharedSecret(agentConfig); err != nil {
			return errors.Trace(err)
		}
		agentConfig = a.CurrentConfig() // ensureMongoSharedSecret may have updated the config

		mongoInfo, ok := agentConfig.MongoInfo()
		if !ok {
			return errors.New("unable to retrieve mongo info to check replicaset")
		}

		needReplicasetInit, err = isReplicasetInitNeeded(mongoInfo)
		if err != nil {
			return errors.Annotate(err, "error while checking replicaset")
		}

		// If the replicaset is to be initialised the machine addresses
		// need to be retrieved *before* MongoDB is restarted with the
		// --replset option (in EnsureMongoServer). Once MongoDB is
		// started with --replset it won't respond to queries until the
		// replicaset is initiated.
		if needReplicasetInit {
			logger.Infof("replicaset not yet configured")
			machineAddrs, err = getMachineAddresses(agentConfig)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}

	// EnsureMongoServer installs/upgrades the init config as necessary.
	ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	if err := cmdutil.EnsureMongoServer(ensureServerParams); err != nil {
		return err
	}

	// Initiate the replicaset if required.
	if needReplicasetInit {
		servingInfo, ok := agentConfig.StateServingInfo()
		if !ok {
			return stateWorkerServingConfigErr
		}
		mongoInfo, ok := agentConfig.MongoInfo()
		if !ok {
			return errors.New("unable to retrieve mongo info to initiate replicaset")
		}
		if err := initiateReplicaSet(mongoInfo, servingInfo.StatePort, machineAddrs); err != nil {
			return err
		}
	}

	return nil
}

// ensureMongoAdminUser ensures that the machine's mongo user is in
// the admin DB.
func (a *MachineAgent) ensureMongoAdminUser(agentConfig agent.Config) (added bool, err error) {
	mongoInfo, ok1 := agentConfig.MongoInfo()
	servingInfo, ok2 := agentConfig.StateServingInfo()
	if !ok1 || !ok2 {
		return false, stateWorkerServingConfigErr
	}
	dialInfo, err := mongo.DialInfo(mongoInfo.Info, mongo.DefaultDialOpts())
	if err != nil {
		return false, err
	}
	if len(dialInfo.Addrs) > 1 {
		logger.Infof("more than one state server; admin user must exist")
		return false, nil
	}
	return ensureMongoAdminUser(mongo.EnsureAdminUserParams{
		DialInfo:  dialInfo,
		Namespace: agentConfig.Value(agent.Namespace),
		DataDir:   agentConfig.DataDir(),
		Port:      servingInfo.StatePort,
		User:      mongoInfo.Tag.String(),
		Password:  mongoInfo.Password,
	})
}

// ensureMongoSharedSecret generates a MongoDB shared secret if
// required, updating the agent's config and state.
func (a *MachineAgent) ensureMongoSharedSecret(agentConfig agent.Config) error {
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return stateWorkerServingConfigErr
	}

	if servingInfo.SharedSecret != "" {
		return nil // Already done
	}

	logger.Infof("state serving info has no shared secret - generating")

	var err error
	servingInfo.SharedSecret, err = mongo.GenerateSharedSecret()
	if err != nil {
		return err
	}
	logger.Debugf("updating state serving info in agent config")
	if err = a.ChangeConfig(func(config agent.ConfigSetter) error {
		config.SetStateServingInfo(servingInfo)
		return nil
	}); err != nil {
		return err
	}
	agentConfig = a.CurrentConfig()

	logger.Debugf("updating state serving info in state")

	// Note: we set Direct=true in the mongo options because it's
	// possible that we've previously upgraded the mongo server's
	// configuration to form a replicaset, but failed to initiate it.
	dialOpts := mongo.DefaultDialOpts()
	dialOpts.Direct = true
	st, _, err := openState(agentConfig, dialOpts)
	if err != nil {
		return err
	}
	defer st.Close()

	ssi := cmdutil.ParamsStateServingInfoToStateStateServingInfo(servingInfo)
	if err := st.SetStateServingInfo(ssi); err != nil {
		return errors.Errorf("cannot set state serving info: %v", err)
	}

	logger.Infof("shared secret updated in state serving info")
	return nil
}

// isReplicasetInitNeeded returns true if the replicaset needs to be
// initiated.
func isReplicasetInitNeeded(mongoInfo *mongo.MongoInfo) (bool, error) {
	dialInfo, err := mongo.DialInfo(mongoInfo.Info, mongo.DefaultDialOpts())
	if err != nil {
		return false, errors.Annotate(err, "cannot generate dial info to check replicaset")
	}
	dialInfo.Username = mongoInfo.Tag.String()
	dialInfo.Password = mongoInfo.Password

	session, err := mgo.DialWithInfo(dialInfo)
	if err != nil {
		return false, errors.Annotate(err, "cannot dial mongo to check replicaset")
	}
	defer session.Close()

	cfg, err := replicaset.CurrentConfig(session)
	if err != nil {
		logger.Debugf("couldn't retrieve replicaset config (not fatal): %v", err)
		return true, nil
	}
	numMembers := len(cfg.Members)
	logger.Debugf("replicaset member count: %d", numMembers)
	return numMembers < 1, nil
}

// getMachineAddresses connects to state to determine the machine's
// network addresses.
func getMachineAddresses(agentConfig agent.Config) ([]network.Address, error) {
	logger.Debugf("opening state to get machine addresses")
	dialOpts := mongo.DefaultDialOpts()
	dialOpts.Direct = true
	st, m, err := openState(agentConfig, dialOpts)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open state to retrieve machine addresses")
	}
	defer st.Close()
	return m.Addresses(), nil
}

// initiateReplicaSet connects to MongoDB and sets up the replicaset.
func initiateReplicaSet(mongoInfo *mongo.MongoInfo, statePort int, machineAddrs []network.Address) error {
	peerAddr := mongo.SelectPeerAddress(machineAddrs)
	if peerAddr == "" {
		return errors.Errorf("no appropriate peer address found in %q", machineAddrs)
	}

	dialInfo, err := mongo.DialInfo(mongoInfo.Info, mongo.DefaultDialOpts())
	if err != nil {
		return errors.Annotate(err, "cannot generate dial info to initiate replicaset")
	}

	if err := maybeInitiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: net.JoinHostPort(peerAddr, fmt.Sprint(statePort)),
		User:           mongoInfo.Tag.String(), // TODO(dfc) InitiateMongoParams should take a Tag
		Password:       mongoInfo.Password,
	}); err != nil && err != peergrouper.ErrReplicaSetAlreadyInitiated {
		return err
	}
	return nil
}

func openState(agentConfig agent.Config, dialOpts mongo.DialOpts) (_ *state.State, _ *state.Machine, err error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, nil, fmt.Errorf("no state info available")
	}
	st, err := state.Open(agentConfig.Environment(), info, dialOpts, environs.NewStatePolicy())
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
		for _, ch := range []chan struct{}{
			a.upgradeWorkerContext.UpgradeComplete,
			a.initialAgentUpgradeCheckComplete,
		} {
			select {
			case <-stop:
				return nil
			case <-ch:
			}
		}
		logger.Debugf("upgrades done, starting worker %q", name)

		// For windows clients we need to make sure we set a random password in a
		// registry file and use that password for the jujud user and its services
		// before starting anything else.
		// Services on windows need to know the user's password to start up. The only
		// way to store that password securely is if the user running the services
		// sets the password. This cannot be done during cloud-init so it is done here.
		// This needs to get ran in between finishing the upgrades and starting
		// the rest of the workers(in particular the deployer which should use
		// the new password)
		if err := password.EnsureJujudPassword(); err != nil {
			return errors.Annotate(err, "Could not ensure jujud password")
		}

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

func (a *MachineAgent) setMachineStatus(apiState api.Connection, status params.Status, info string) error {
	tag := a.Tag().(names.MachineTag)
	machine, err := apiState.Machiner().Machine(tag)
	if err != nil {
		return errors.Trace(err)
	}
	if err := machine.SetStatus(status, info, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WorkersStarted returns a channel that's closed once all top level workers
// have been started. This is provided for testing purposes.
func (a *MachineAgent) WorkersStarted() <-chan struct{} {
	return a.workersStarted
}

func (a *MachineAgent) Tag() names.Tag {
	return names.NewMachineTag(a.machineId)
}

func (a *MachineAgent) createJujuRun(dataDir string) error {
	// TODO do not remove the symlink if it already points
	// to the right place.
	if err := os.Remove(JujuRun); err != nil && !os.IsNotExist(err) {
		return err
	}
	jujud := filepath.Join(dataDir, "tools", a.Tag().String(), jujunames.Jujud)
	return symlink.New(jujud, JujuRun)
}

func (a *MachineAgent) uninstallAgent(agentConfig agent.Config) error {
	var errors []error
	agentServiceName := agentConfig.Value(agent.AgentServiceName)
	if agentServiceName == "" {
		// For backwards compatibility, handle lack of AgentServiceName.
		agentServiceName = os.Getenv("UPSTART_JOB")
	}
	if agentServiceName != "" {
		svc, err := service.DiscoverService(agentServiceName, common.Conf{})
		if err != nil {
			errors = append(errors, fmt.Errorf("cannot remove service %q: %v", agentServiceName, err))
		} else if err := svc.Remove(); err != nil {
			errors = append(errors, fmt.Errorf("cannot remove service %q: %v", agentServiceName, err))
		}
	}

	// Remove the juju-run symlink.
	if err := os.Remove(JujuRun); err != nil && !os.IsNotExist(err) {
		errors = append(errors, err)
	}

	insideLXC, err := lxcutils.RunningInsideLXC()
	if err != nil {
		errors = append(errors, err)
	} else if insideLXC {
		// We're running inside LXC, so loop devices may leak. Detach
		// any loop devices that are backed by files on this machine.
		//
		// It is necessary to do this here as well as in container/lxc,
		// as container/lxc needs to check in the container's rootfs
		// to see if the loop device is attached to the container; that
		// will fail if the data-dir is removed first.
		if err := a.loopDeviceManager.DetachLoopDevices("/", agentConfig.DataDir()); err != nil {
			errors = append(errors, err)
		}
	}

	namespace := agentConfig.Value(agent.Namespace)
	if err := mongo.RemoveService(namespace); err != nil {
		errors = append(errors, fmt.Errorf("cannot stop/remove mongo service with namespace %q: %v", namespace, err))
	}
	if err := os.RemoveAll(agentConfig.DataDir()); err != nil {
		errors = append(errors, err)
	}
	if len(errors) == 0 {
		return nil
	}
	return fmt.Errorf("uninstall failed: %v", errors)
}

func newConnRunner(conns ...cmdutil.Pinger) worker.Runner {
	return worker.NewRunner(cmdutil.ConnectionIsFatal(logger, conns...), cmdutil.MoreImportant)
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

func metricAPI(st api.Connection) metricsmanager.MetricsManagerClient {
	return metricsmanager.NewClient(st)
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, agentConfig agent.Config) deployer.Context {
	return deployer.NewSimpleContext(agentConfig, st)
}
