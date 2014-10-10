// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
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
	"github.com/juju/utils"
	"github.com/juju/utils/symlink"
	"github.com/juju/utils/voyeur"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	jujunames "github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider"
	"github.com/juju/juju/replicaset"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/charmrevisionworker"
	"github.com/juju/juju/worker/cleaner"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/firewaller"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/localstorage"
	workerlogger "github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/machineenvironmentworker"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/metricworker"
	"github.com/juju/juju/worker/minunitsworker"
	"github.com/juju/juju/worker/networker"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/terminationworker"
	"github.com/juju/juju/worker/upgrader"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

var newRunner = worker.NewRunner

const bootstrapMachineId = "0"

// eitherState can be either a *state.State or a *api.State.
type eitherState interface{}

var (
	retryDelay      = 3 * time.Second
	jujuRun         = paths.MustSucceed(paths.JujuRun(version.Current.Series))
	useMultipleCPUs = utils.UseMultipleCPUs

	// The following are defined as variables to
	// allow the tests to intercept calls to the functions.
	ensureMongoServer        = mongo.EnsureServer
	maybeInitiateMongoServer = peergrouper.MaybeInitiateMongoServer
	ensureMongoAdminUser     = mongo.EnsureAdminUser
	newSingularRunner        = singular.New
	peergrouperNew           = peergrouper.New
	newNetworker             = networker.NewNetworker
	newFirewaller            = firewaller.NewFirewaller

	// reportOpenedAPI is exposed for tests to know when
	// the State has been successfully opened.
	reportOpenedState = func(eitherState) {}

	// reportOpenedAPI is exposed for tests to know when
	// the API has been successfully opened.
	reportOpenedAPI = func(eitherState) {}

	getMetricAPI = metricAPI
)

// IsRestorePreparing returns bool representing if we are in restore mode
// but not running restore
func (a *MachineAgent) IsRestorePreparing() bool {
	return a.restoreContext.restorePreparing()
}

func (a *MachineAgent) IsRestoreRunning() bool {
	return a.restoreContext.restoreRunning()
}

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
	MachineId            string
	previousAgentVersion version.Number
	runner               worker.Runner
	configChangedVal     voyeur.Value
	upgradeWorkerContext *upgradeWorkerContext
	restoreContext       *restoreContext
	restoreMode          bool
	restoring            bool
	workersStarted       chan struct{}
	st                   *state.State

	mongoInitMutex   sync.Mutex
	mongoInitialized bool
}

// Info returns usage information for the command.
func (a *MachineAgent) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "machine",
		Purpose: "run a juju machine agent",
	}
}

func (a *MachineAgent) SetFlags(f *gnuflag.FlagSet) {
	a.AgentConf.AddFlags(f)
	f.StringVar(&a.MachineId, "machine-id", "", "id of the machine to run")
}

// Init initializes the command for running.
func (a *MachineAgent) Init(args []string) error {
	if !names.IsValidMachine(a.MachineId) {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = newRunner(isFatal, moreImportant)
	a.workersStarted = make(chan struct{})
	a.upgradeWorkerContext = NewUpgradeWorkerContext()
	a.restoreContext = NewRestoreContext(&a.tomb)
	return nil
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

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	// Due to changes in the logging, and needing to care about old
	// environments that have been upgraded, we need to explicitly remove the
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	loggo.RemoveWriter("logfile")
	defer a.tomb.Done()
	if err := a.ReadConfig(a.Tag().String()); err != nil {
		return fmt.Errorf("cannot read agent configuration: %v", err)
	}
	agentConfig := a.CurrentConfig()
	if err := setupLogging(agentConfig); err != nil {
		return err
	}
	logger.Infof("machine agent %v start (%s [%s])", a.Tag(), version.Current, runtime.Compiler)

	if err := a.upgradeWorkerContext.InitializeUsingAgent(a); err != nil {
		return errors.Annotate(err, "error during upgradeWorkerContext initialisation")
	}
	a.configChangedVal.Set(struct{}{})
	a.previousAgentVersion = agentConfig.UpgradedToVersion()
	network.InitializeFromConfig(agentConfig)
	charm.CacheDir = filepath.Join(agentConfig.DataDir(), "charmcache")
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
	if err == worker.ErrTerminateAgent {
		err = a.uninstallAgent(agentConfig)
	}
	err = agentDone(err)
	a.tomb.Kill(err)
	return err
}

func (a *MachineAgent) ChangeConfig(mutate AgentConfigMutator) error {
	err := a.AgentConf.ChangeConfig(mutate)
	a.configChangedVal.Set(struct{}{})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (a *MachineAgent) newRestoreStateWatcher(st *state.State) (worker.Worker, error) {
	rWorker := func(stopch <-chan struct{}) error {
		return a.restoreStateWatcher(st, stopch)
	}
	return worker.NewSimpleWorker(rWorker), nil
}

func (a *MachineAgent) restoreChanged(st *state.State) error {
	rinfo, err := st.EnsureRestoreInfo()
	if err != nil {
		return errors.Annotate(err, "cannot read restore state")
	}
	switch rinfo.Status() {
	case state.RestorePending:
		a.PrepareRestore()
	case state.RestoreInProgress:
		a.BeginRestore()
	}
	return nil
}

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
func (a *MachineAgent) APIWorker() (worker.Worker, error) {
	agentConfig := a.CurrentConfig()
	st, entity, err := openAPIState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	reportOpenedAPI(st)

	// Check if the network management is disabled.
	envConfig, err := st.Environment().EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot read environment config: %v", err)
	}
	disableNetworkManagement, _ := envConfig.DisableNetworkManagement()
	if disableNetworkManagement {
		logger.Infof("network management is disabled")
	}
	// Check if firewall-mode is "none" to disable the firewaller.
	firewallMode := envConfig.FirewallMode()
	disableFirewaller := firewallMode == config.FwNone

	// Refresh the configuration, since it may have been updated after opening state.
	agentConfig = a.CurrentConfig()
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

	rsyslogMode := rsyslog.RsyslogModeForwarding
	runner := newRunner(connectionIsFatal(st), moreImportant)
	var singularRunner worker.Runner
	for _, job := range entity.Jobs() {
		if job == params.JobManageEnviron {
			rsyslogMode = rsyslog.RsyslogModeAccumulate
			conn := singularAPIConn{st, st.Agent()}
			singularRunner, err = newSingularRunner(runner, conn)
			if err != nil {
				return nil, fmt.Errorf("cannot make singular API Runner: %v", err)
			}
			break
		}
	}

	// Before starting any workers, ensure we record the Juju version this machine
	// agent is running.
	currentTools := &coretools.Tools{Version: version.Current}
	if err := st.Upgrader().SetVersion(agentConfig.Tag().String(), currentTools.Version); err != nil {
		return nil, errors.Annotate(err, "cannot set machine agent version")
	}

	providerType := agentConfig.Value(agent.ProviderType)

	// Run the upgrader and the upgrade-steps worker without waiting for
	// the upgrade steps to complete.
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewUpgrader(
			st.Upgrader(),
			agentConfig,
			a.previousAgentVersion,
			a.upgradeWorkerContext.IsUpgradeRunning,
		), nil
	})
	runner.StartWorker("upgrade-steps", func() (worker.Worker, error) {
		return a.upgradeWorkerContext.Worker(a, st, entity.Jobs()), nil
	})

	// All other workers must wait for the upgrade steps to complete
	// before starting.
	a.startWorkerAfterUpgrade(runner, "machiner", func() (worker.Worker, error) {
		return machiner.NewMachiner(st.Machiner(), agentConfig), nil
	})
	a.startWorkerAfterUpgrade(runner, "apiaddressupdater", func() (worker.Worker, error) {
		return apiaddressupdater.NewAPIAddressUpdater(st.Machiner(), a), nil
	})
	a.startWorkerAfterUpgrade(runner, "logger", func() (worker.Worker, error) {
		return workerlogger.NewLogger(st.Logger(), agentConfig), nil
	})
	a.startWorkerAfterUpgrade(runner, "machineenvironmentworker", func() (worker.Worker, error) {
		return machineenvironmentworker.NewMachineEnvironmentWorker(st.Environment(), agentConfig), nil
	})
	a.startWorkerAfterUpgrade(runner, "rsyslog", func() (worker.Worker, error) {
		return newRsyslogConfigWorker(st.Rsyslog(), agentConfig, rsyslogMode)
	})

	// Start networker depending on configuration and job.
	intrusiveMode := false
	for _, job := range entity.Jobs() {
		if job == params.JobManageNetworking {
			intrusiveMode = true
			break
		}
	}
	intrusiveMode = intrusiveMode && !disableNetworkManagement
	a.startWorkerAfterUpgrade(runner, "networker", func() (worker.Worker, error) {
		return newNetworker(st.Networker(), agentConfig, intrusiveMode, networker.DefaultConfigBaseDir)
	})

	// If not a local provider bootstrap machine, start the worker to
	// manage SSH keys.
	if providerType != provider.Local || a.MachineId != bootstrapMachineId {
		a.startWorkerAfterUpgrade(runner, "authenticationworker", func() (worker.Worker, error) {
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
		case params.JobHostUnits:
			a.startWorkerAfterUpgrade(runner, "deployer", func() (worker.Worker, error) {
				apiDeployer := st.Deployer()
				context := newDeployContext(apiDeployer, agentConfig)
				return deployer.NewDeployer(apiDeployer, context), nil
			})
		case params.JobManageEnviron:
			a.startWorkerAfterUpgrade(singularRunner, "environ-provisioner", func() (worker.Worker, error) {
				return provisioner.NewEnvironProvisioner(st.Provisioner(), agentConfig), nil
			})
			// TODO(axw) 2013-09-24 bug #1229506
			// Make another job to enable the firewaller. Not all
			// environments are capable of managing ports
			// centrally.
			if !disableFirewaller {
				a.startWorkerAfterUpgrade(singularRunner, "firewaller", func() (worker.Worker, error) {
					return newFirewaller(st.Firewaller())
				})
			} else {
				logger.Debugf("not starting firewaller worker - firewall-mode is %q", config.FwNone)
			}
			a.startWorkerAfterUpgrade(singularRunner, "charm-revision-updater", func() (worker.Worker, error) {
				return charmrevisionworker.NewRevisionUpdateWorker(st.CharmRevisionUpdater()), nil
			})

			logger.Infof("starting metric workers")
			a.startWorkerAfterUpgrade(runner, "metriccleanupworker", func() (worker.Worker, error) {
				return metricworker.NewCleanup(getMetricAPI(st)), nil
			})
			a.startWorkerAfterUpgrade(runner, "metricsenderworker", func() (worker.Worker, error) {
				return metricworker.NewSender(getMetricAPI(st)), nil
			})
		case params.JobManageStateDeprecated:
			// Legacy environments may set this, but we ignore it.
		default:
			// TODO(dimitern): Once all workers moved over to using
			// the API, report "unknown job type" here.
		}
	}
	return newCloseWorker(runner, st), nil // Note: a worker.Runner is itself a worker.Worker.
}

// setupContainerSupport determines what containers can be run on this machine and
// initialises suitable infrastructure to support such containers.
func (a *MachineAgent) setupContainerSupport(runner worker.Runner, st *api.State, entity *apiagent.Entity, agentConfig agent.Config) error {
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
	st *api.State,
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
	initLock, err := hookExecutionLock(agentConfig.DataDir())
	if err != nil {
		return err
	}
	// Start the watcher to fire when a container is first requested on the machine.
	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	handler := provisioner.NewContainerSetupHandler(
		runner,
		watcherName,
		containers,
		machine,
		pr,
		agentConfig,
		initLock,
	)
	a.startWorkerAfterUpgrade(runner, watcherName, func() (worker.Worker, error) {
		return worker.NewStringsWorker(handler), nil
	})
	return nil
}

// StateWorker returns a worker running all the workers that require
// a *state.State connection.
func (a *MachineAgent) StateWorker() (worker.Worker, error) {
	agentConfig := a.CurrentConfig()

	// Create system-identity file.
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return nil, err
	}

	// Start MongoDB server and dial.
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}
	st, m, err := openState(agentConfig, stateWorkerDialOpts)
	if err != nil {
		return nil, err
	}
	reportOpenedState(st)
	registerSimplestreamsDataSource(st.Storage())

	singularStateConn := singularStateConn{st.MongoSession(), m}
	runner := newRunner(connectionIsFatal(st), moreImportant)
	singularRunner, err := newSingularRunner(runner, singularStateConn)
	if err != nil {
		return nil, fmt.Errorf("cannot make singular State Runner: %v", err)
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
			a.startWorkerAfterUpgrade(runner, "instancepoller", func() (worker.Worker, error) {
				return instancepoller.NewWorker(st), nil
			})
			a.startWorkerAfterUpgrade(runner, "peergrouper", func() (worker.Worker, error) {
				return peergrouperNew(st)
			})
			a.startWorkerAfterUpgrade(runner, "restore", func() (worker.Worker, error) {
				return a.newRestoreStateWatcher(st)
			})

			runner.StartWorker("apiserver", func() (worker.Worker, error) {
				// If the configuration does not have the required information,
				// it is currently not a recoverable error, so we kill the whole
				// agent, potentially enabling human intervention to fix
				// the agent's configuration file. In the future, we may retrieve
				// the state server certificate and key from the state, and
				// this should then change.
				info, ok := agentConfig.StateServingInfo()
				if !ok {
					return nil, &fatalError{"StateServingInfo not available and we need it"}
				}
				cert := []byte(info.Cert)
				key := []byte(info.PrivateKey)

				if len(cert) == 0 || len(key) == 0 {
					return nil, &fatalError{"configuration does not have state server cert/key"}
				}
				dataDir := agentConfig.DataDir()
				logDir := agentConfig.LogDir()

				endpoint := net.JoinHostPort("", strconv.Itoa(info.APIPort))
				listener, err := net.Listen("tcp", endpoint)
				if err != nil {
					return nil, err
				}
				return apiserver.NewServer(st, listener, apiserver.ServerConfig{
					Cert:           cert,
					Key:            key,
					DataDir:        dataDir,
					LogDir:         logDir,
					Validator:      a.limitLogins,
					RestoreContext: a.restoreContext,
				})
			})
			a.startWorkerAfterUpgrade(singularRunner, "cleaner", func() (worker.Worker, error) {
				return cleaner.NewCleaner(st), nil
			})
			a.startWorkerAfterUpgrade(singularRunner, "resumer", func() (worker.Worker, error) {
				// The action of resumer is so subtle that it is not tested,
				// because we can't figure out how to do so without brutalising
				// the transaction log.
				return resumer.NewResumer(st), nil
			})
			a.startWorkerAfterUpgrade(singularRunner, "minunitsworker", func() (worker.Worker, error) {
				return minunitsworker.NewMinUnitsWorker(st), nil
			})
		case state.JobManageStateDeprecated:
			// Legacy environments may set this, but we ignore it.
		default:
			logger.Warningf("ignoring unknown job %q", job)
		}
	}
	return newCloseWorker(runner, st), nil
}

// stateWorkerDialOpts is a mongo.DialOpts suitable
// for use by StateWorker to dial mongo.
//
// This must be overridden in tests, as it assumes
// journaling is enabled.
var stateWorkerDialOpts mongo.DialOpts

func init() {
	stateWorkerDialOpts = mongo.DefaultDialOpts()
	stateWorkerDialOpts.PostDial = func(session *mgo.Session) error {
		safe := mgo.Safe{
			// Wait for group commit if journaling is enabled,
			// which is always true in production.
			J: true,
		}
		_, err := replicaset.CurrentConfig(session)
		if err == nil {
			// set mongo to write-majority (writes only returned after
			// replicated to a majority of replica-set members).
			safe.WMode = "majority"
		}
		session.SetSafe(&safe)
		return nil
	}
}

// limitLogins is called by the API server for each login attempt.
// it returns an error if upgrads or restore are running.
func (a *MachineAgent) limitLogins(req params.LoginRequest) error {
	err := a.limitLoginsDuringRestore(req)
	if err != nil {
		return err
	}
	err = a.limitLoginsDuringUpgrade(req)
	if err != nil {
		return err
	}
	return nil
}

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
	if a.upgradeWorkerContext.IsUpgradeRunning() {
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
		return errors.Errorf("login for %q blocked because upgrade is in progress", authTag)
	} else {
		return nil // allow all logins
	}
}

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

	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("state worker was started with no state serving info")
	}

	// When upgrading from a pre-HA-capable environment,
	// we must add machine-0 to the admin database and
	// initiate its replicaset.
	//
	// TODO(axw) remove this when we no longer need
	// to upgrade from pre-HA-capable environments.
	var shouldInitiateMongoServer bool
	var addrs []network.Address
	if isPreHAVersion(a.previousAgentVersion) {
		_, err := a.ensureMongoAdminUser(agentConfig)
		if err != nil {
			return err
		}
		if servingInfo.SharedSecret == "" {
			servingInfo.SharedSecret, err = mongo.GenerateSharedSecret()
			if err != nil {
				return err
			}
			if err = a.ChangeConfig(func(config agent.ConfigSetter) error {
				config.SetStateServingInfo(servingInfo)
				return nil
			}); err != nil {
				return err
			}
			agentConfig = a.CurrentConfig()
		}
		// Note: we set Direct=true in the mongo options because it's
		// possible that we've previously upgraded the mongo server's
		// configuration to form a replicaset, but failed to initiate it.
		st, m, err := openState(agentConfig, mongo.DialOpts{Direct: true})
		if err != nil {
			return err
		}
		ssi := paramsStateServingInfoToStateStateServingInfo(servingInfo)
		if err := st.SetStateServingInfo(ssi); err != nil {
			st.Close()
			return fmt.Errorf("cannot set state serving info: %v", err)
		}
		st.Close()
		addrs = m.Addresses()
		shouldInitiateMongoServer = true
	}

	// ensureMongoServer installs/upgrades the upstart config as necessary.
	ensureServerParams, err := newEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	if err := ensureMongoServer(ensureServerParams); err != nil {
		return err
	}
	if !shouldInitiateMongoServer {
		return nil
	}

	// Initiate the replicaset for upgraded environments.
	//
	// TODO(axw) remove this when we no longer need
	// to upgrade from pre-HA-capable environments.
	stateInfo, ok := agentConfig.MongoInfo()
	if !ok {
		return fmt.Errorf("state worker was started with no state serving info")
	}
	dialInfo, err := mongo.DialInfo(stateInfo.Info, mongo.DefaultDialOpts())
	if err != nil {
		return err
	}
	peerAddr := mongo.SelectPeerAddress(addrs)
	if peerAddr == "" {
		return fmt.Errorf("no appropriate peer address found in %q", addrs)
	}
	if err := maybeInitiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: net.JoinHostPort(peerAddr, fmt.Sprint(servingInfo.StatePort)),
		// TODO(dfc) InitiateMongoParams should take a Tag
		User:     stateInfo.Tag.String(),
		Password: stateInfo.Password,
	}); err != nil {
		return err
	}
	return nil
}

func paramsStateServingInfoToStateStateServingInfo(i params.StateServingInfo) state.StateServingInfo {
	return state.StateServingInfo{
		APIPort:        i.APIPort,
		StatePort:      i.StatePort,
		Cert:           i.Cert,
		PrivateKey:     i.PrivateKey,
		SharedSecret:   i.SharedSecret,
		SystemIdentity: i.SystemIdentity,
	}
}

func (a *MachineAgent) ensureMongoAdminUser(agentConfig agent.Config) (added bool, err error) {
	stateInfo, ok1 := agentConfig.MongoInfo()
	servingInfo, ok2 := agentConfig.StateServingInfo()
	if !ok1 || !ok2 {
		return false, fmt.Errorf("no state serving info configuration")
	}
	dialInfo, err := mongo.DialInfo(stateInfo.Info, mongo.DefaultDialOpts())
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
		User:      stateInfo.Tag.String(),
		Password:  stateInfo.Password,
	})
}

func isPreHAVersion(v version.Number) bool {
	return v.Compare(version.MustParse("1.19.0")) < 0
}

func openState(agentConfig agent.Config, dialOpts mongo.DialOpts) (_ *state.State, _ *state.Machine, err error) {
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, nil, fmt.Errorf("no state info available")
	}
	st, err := state.Open(info, dialOpts, environs.NewStatePolicy())
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
		return a.upgradeWaiterWorker(start), nil
	})
}

// upgradeWaiterWorker runs the specified worker after upgrades have completed.
func (a *MachineAgent) upgradeWaiterWorker(start func() (worker.Worker, error)) worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		// Wait for the upgrade to complete (or for us to be stopped).
		select {
		case <-stop:
			return nil
		case <-a.upgradeWorkerContext.UpgradeComplete:
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
			return err
		case <-stop:
			worker.Kill()
		}
		return <-waitCh // Ensure worker has stopped before returning.
	})
}

func (a *MachineAgent) setMachineStatus(apiState *api.State, status params.Status, info string) error {
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
	return names.NewMachineTag(a.MachineId)
}

func (a *MachineAgent) createJujuRun(dataDir string) error {
	// TODO do not remove the symlink if it already points
	// to the right place.
	if err := os.Remove(jujuRun); err != nil && !os.IsNotExist(err) {
		return err
	}
	jujud := filepath.Join(dataDir, "tools", a.Tag().String(), jujunames.Jujud)
	return symlink.New(jujud, jujuRun)
}

func (a *MachineAgent) uninstallAgent(agentConfig agent.Config) error {
	var errors []error
	agentServiceName := agentConfig.Value(agent.AgentServiceName)
	if agentServiceName == "" {
		// For backwards compatibility, handle lack of AgentServiceName.
		agentServiceName = os.Getenv("UPSTART_JOB")
	}
	if agentServiceName != "" {
		if err := service.NewService(agentServiceName, common.Conf{}).Remove(); err != nil {
			errors = append(errors, fmt.Errorf("cannot remove service %q: %v", agentServiceName, err))
		}
	}
	// Remove the juju-run symlink.
	if err := os.Remove(jujuRun); err != nil && !os.IsNotExist(err) {
		errors = append(errors, err)
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

// singularAPIConn implements singular.Conn on
// top of an API connection.
type singularAPIConn struct {
	apiState   *api.State
	agentState *apiagent.State
}

func (c singularAPIConn) IsMaster() (bool, error) {
	return c.agentState.IsMaster()
}

func (c singularAPIConn) Ping() error {
	return c.apiState.Ping()
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

func metricAPI(st *api.State) metricsmanager.MetricsManagerClient {
	return metricsmanager.NewClient(st)
}
