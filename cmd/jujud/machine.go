// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"labix.org/v2/mgo"
	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/mongo"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/container/kvm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apiagent "launchpad.net/juju-core/state/api/agent"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/upgrades"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/voyeur"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/apiaddressupdater"
	"launchpad.net/juju-core/worker/authenticationworker"
	"launchpad.net/juju-core/worker/charmrevisionworker"
	"launchpad.net/juju-core/worker/cleaner"
	"launchpad.net/juju-core/worker/deployer"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/instancepoller"
	"launchpad.net/juju-core/worker/localstorage"
	workerlogger "launchpad.net/juju-core/worker/logger"
	"launchpad.net/juju-core/worker/machineenvironmentworker"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/juju-core/worker/minunitsworker"
	"launchpad.net/juju-core/worker/peergrouper"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/juju-core/worker/resumer"
	"launchpad.net/juju-core/worker/rsyslog"
	"launchpad.net/juju-core/worker/singular"
	"launchpad.net/juju-core/worker/terminationworker"
	"launchpad.net/juju-core/worker/upgrader"
)

var logger = loggo.GetLogger("juju.cmd.jujud")

var newRunner = worker.NewRunner

const bootstrapMachineId = "0"

// eitherState can be either a *state.State or a *api.State.
type eitherState interface{}

var (
	retryDelay      = 3 * time.Second
	jujuRun         = "/usr/local/bin/juju-run"
	useMultipleCPUs = utils.UseMultipleCPUs

	// The following are defined as variables to
	// allow the tests to intercept calls to the functions.
	ensureMongoServer        = mongo.EnsureServer
	maybeInitiateMongoServer = peergrouper.MaybeInitiateMongoServer
	ensureMongoAdminUser     = mongo.EnsureAdminUser
	newSingularRunner        = singular.New
	peergrouperNew           = peergrouper.New

	// reportOpenedAPI is exposed for tests to know when
	// the State has been successfully opened.
	reportOpenedState = func(eitherState) {}

	// reportOpenedAPI is exposed for tests to know when
	// the API has been successfully opened.
	reportOpenedAPI = func(eitherState) {}
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	cmd.CommandBase
	tomb tomb.Tomb
	AgentConf
	MachineId        string
	runner           worker.Runner
	configChangedVal voyeur.Value
	upgradeComplete  chan struct{}
	workersStarted   chan struct{}
	st               *state.State
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
	if !names.IsMachine(a.MachineId) {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	if err := a.AgentConf.CheckArgs(args); err != nil {
		return err
	}
	a.runner = newRunner(isFatal, moreImportant)
	a.upgradeComplete = make(chan struct{})
	a.workersStarted = make(chan struct{})
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

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	// Due to changes in the logging, and needing to care about old
	// environments that have been upgraded, we need to explicitly remove the
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	loggo.RemoveWriter("logfile")
	defer a.tomb.Done()
	logger.Infof("machine agent %v start (%s [%s])", a.Tag(), version.Current, runtime.Compiler)
	if err := a.ReadConfig(a.Tag()); err != nil {
		return fmt.Errorf("cannot read agent configuration: %v", err)
	}
	a.configChangedVal.Set(struct{}{})
	agentConfig := a.CurrentConfig()
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

func (a *MachineAgent) ChangeConfig(mutate func(config agent.ConfigSetter)) error {
	err := a.AgentConf.ChangeConfig(mutate)
	a.configChangedVal.Set(struct{}{})
	return err
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

	// Refresh the configuration, since it may have been updated after opening state.
	agentConfig = a.CurrentConfig()

	for _, job := range entity.Jobs() {
		if job.NeedsState() {
			info, err := st.Agent().StateServingInfo()
			if err != nil {
				return nil, fmt.Errorf("cannot get state serving info: %v", err)
			}
			err = a.ChangeConfig(func(config agent.ConfigSetter) {
				config.SetStateServingInfo(info)
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

	// Run the upgrader and the upgrade-steps worker without waiting for
	// the upgrade steps to complete.
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewUpgrader(st.Upgrader(), agentConfig), nil
	})
	runner.StartWorker("upgrade-steps", func() (worker.Worker, error) {
		return a.upgradeWorker(st, entity.Jobs(), agentConfig), nil
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

	// If not a local provider bootstrap machine, start the worker to
	// manage SSH keys.
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType != provider.Local || a.MachineId != bootstrapMachineId {
		a.startWorkerAfterUpgrade(runner, "authenticationworker", func() (worker.Worker, error) {
			return authenticationworker.NewWorker(st.KeyUpdater(), agentConfig), nil
		})
	}

	// Perform the operations needed to set up hosting for containers.
	if err := a.setupContainerSupport(runner, st, entity, agentConfig); err != nil {
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
			a.startWorkerAfterUpgrade(singularRunner, "firewaller", func() (worker.Worker, error) {
				return firewaller.NewFirewaller(st.Firewaller())
			})
			a.startWorkerAfterUpgrade(singularRunner, "charm-revision-updater", func() (worker.Worker, error) {
				return charmrevisionworker.NewRevisionUpdateWorker(st.CharmRevisionUpdater()), nil
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
	// We don't yet support nested lxc containers but anything else can run an LXC container.
	if entity.ContainerType() != instance.LXC {
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
	tag string,
	containers []instance.ContainerType,
	agentConfig agent.Config,
) error {
	pr := st.Provisioner()
	machine, err := pr.Machine(tag)
	if err != nil {
		return fmt.Errorf("%s is not in state: %v", tag, err)
	}
	if len(containers) == 0 {
		if err := machine.SupportsNoContainers(); err != nil {
			return fmt.Errorf("clearing supported containers for %s: %v", tag, err)
		}
		return nil
	}
	if err := machine.SetSupportedContainers(containers...); err != nil {
		return fmt.Errorf("setting supported containers for %s: %v", tag, err)
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

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return nil, err
	}

	// Start MondoDB server
	if err := a.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}
	st, m, err := openState(agentConfig)
	if err != nil {
		return nil, err
	}
	reportOpenedState(st)

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
				port := info.APIPort
				cert := []byte(info.Cert)
				key := []byte(info.PrivateKey)

				if len(cert) == 0 || len(key) == 0 {
					return nil, &fatalError{"configuration does not have state server cert/key"}
				}
				dataDir := agentConfig.DataDir()
				logDir := agentConfig.LogDir()
				return apiserver.NewServer(
					st, fmt.Sprintf(":%d", port), cert, key, dataDir, logDir)
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

// ensureMongoServer ensures that mongo is installed and running,
// and ready for opening a state connection.
func (a *MachineAgent) ensureMongoServer(agentConfig agent.Config) error {
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("state worker was started with no state serving info")
	}
	namespace := agentConfig.Value(agent.Namespace)

	// When upgrading from a pre-HA-capable environment,
	// we must add machine-0 to the admin database and
	// initiate its replicaset.
	//
	// TODO(axw) remove this when we no longer need
	// to upgrade from pre-HA-capable environments.
	var shouldInitiateMongoServer bool
	var addrs []instance.Address
	if isPreHAVersion(agentConfig.UpgradedToVersion()) {
		_, err := a.ensureMongoAdminUser(agentConfig)
		if err != nil {
			return err
		}
		if servingInfo.SharedSecret == "" {
			servingInfo.SharedSecret, err = mongo.GenerateSharedSecret()
			if err != nil {
				return err
			}
			if err = a.ChangeConfig(func(config agent.ConfigSetter) {
				config.SetStateServingInfo(servingInfo)
			}); err != nil {
				return err
			}
			agentConfig = a.CurrentConfig()
		}
		st, m, err := openState(agentConfig)
		if err != nil {
			return err
		}
		if err := st.SetStateServingInfo(servingInfo); err != nil {
			st.Close()
			return fmt.Errorf("cannot set state serving info: %v", err)
		}
		st.Close()
		addrs = m.Addresses()
		shouldInitiateMongoServer = true
	}

	// ensureMongoServer installs/upgrades the upstart config as necessary.
	if err := ensureMongoServer(
		agentConfig.DataDir(),
		namespace,
		servingInfo,
	); err != nil {
		return err
	}
	if !shouldInitiateMongoServer {
		return nil
	}

	// Initiate the replicaset for upgraded environments.
	//
	// TODO(axw) remove this when we no longer need
	// to upgrade from pre-HA-capable environments.
	stateInfo, ok := agentConfig.StateInfo()
	if !ok {
		return fmt.Errorf("state worker was started with no state serving info")
	}
	dialInfo, err := state.DialInfo(stateInfo, state.DefaultDialOpts())
	if err != nil {
		return err
	}
	peerAddr := mongo.SelectPeerAddress(addrs)
	if peerAddr == "" {
		return fmt.Errorf("no appropriate peer address found in %q", addrs)
	}
	return maybeInitiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: net.JoinHostPort(peerAddr, fmt.Sprint(servingInfo.StatePort)),
		User:           stateInfo.Tag,
		Password:       stateInfo.Password,
	})
}

func (a *MachineAgent) ensureMongoAdminUser(agentConfig agent.Config) (added bool, err error) {
	stateInfo, ok1 := agentConfig.StateInfo()
	servingInfo, ok2 := agentConfig.StateServingInfo()
	if !ok1 || !ok2 {
		return false, fmt.Errorf("no state serving info configuration")
	}
	dialInfo, err := state.DialInfo(stateInfo, state.DefaultDialOpts())
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
		User:      stateInfo.Tag,
		Password:  stateInfo.Password,
	})
}

func isPreHAVersion(v version.Number) bool {
	return v.Compare(version.MustParse("1.19.0")) < 0
}

func openState(agentConfig agent.Config) (_ *state.State, _ *state.Machine, err error) {
	info, ok := agentConfig.StateInfo()
	if !ok {
		return nil, nil, fmt.Errorf("no state info available")
	}
	st, err := state.Open(info, state.DialOpts{}, environs.NewStatePolicy())
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
		// wait for the upgrade to complete (or for us to be stopped)
		select {
		case <-stop:
			return nil
		case <-a.upgradeComplete:
		}
		w, err := start()
		if err != nil {
			return err
		}
		waitCh := make(chan error)
		go func() {
			waitCh <- w.Wait()
		}()
		select {
		case err := <-waitCh:
			return err
		case <-stop:
			w.Kill()
		}
		return <-waitCh
	})
}

// upgradeWorker runs the required upgrade operations to upgrade to the current Juju version.
func (a *MachineAgent) upgradeWorker(
	apiState *api.State,
	jobs []params.MachineJob,
	agentConfig agent.Config,
) worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		select {
		case <-a.upgradeComplete:
			// Our work is already done (we're probably being restarted
			// because the API connection has gone down), so do nothing.
			<-stop
			return nil
		default:
		}
		// If the machine agent is a state server, flag that state
		// needs to be opened before running upgrade steps
		needsState := false
		for _, job := range jobs {
			if job == params.JobManageEnviron {
				needsState = true
			}
		}
		// We need a *state.State for upgrades. We open it independently
		// of StateWorker, because we have no guarantees about when
		// and how often StateWorker might run.
		var st *state.State
		if needsState {
			var err error
			info, ok := agentConfig.StateInfo()
			if !ok {
				return fmt.Errorf("no state info available")
			}
			st, err = state.Open(info, state.DialOpts{}, environs.NewStatePolicy())
			if err != nil {
				return err
			}
			defer st.Close()
		}
		err := a.runUpgrades(st, apiState, jobs, agentConfig)
		if err != nil {
			return err
		}
		logger.Infof("upgrade to %v completed.", version.Current)
		close(a.upgradeComplete)
		<-stop
		return nil
	})
}

// runUpgrades runs the upgrade operations for each job type and updates the updatedToVersion on success.
func (a *MachineAgent) runUpgrades(
	st *state.State,
	apiState *api.State,
	jobs []params.MachineJob,
	agentConfig agent.Config,
) error {
	from := version.Current
	from.Number = agentConfig.UpgradedToVersion()
	if from == version.Current {
		logger.Infof("upgrade to %v already completed.", version.Current)
		return nil
	}
	var err error
	writeErr := a.ChangeConfig(func(agentConfig agent.ConfigSetter) {
		context := upgrades.NewContext(agentConfig, apiState, st)
		for _, job := range jobs {
			target := upgradeTarget(job)
			if target == "" {
				continue
			}
			logger.Infof("starting upgrade from %v to %v for %v %q", from, version.Current, target, a.Tag())
			if err = upgrades.PerformUpgrade(from.Number, target, context); err != nil {
				err = fmt.Errorf("cannot perform upgrade from %v to %v for %v %q: %v", from, version.Current, target, a.Tag(), err)
				return
			}
		}
		agentConfig.SetUpgradedToVersion(version.Current.Number)
	})
	if writeErr != nil {
		return fmt.Errorf("cannot write updated agent configuration: %v", writeErr)
	}
	return nil
}

func upgradeTarget(job params.MachineJob) upgrades.Target {
	switch job {
	case params.JobManageEnviron:
		return upgrades.StateServer
	case params.JobHostUnits:
		return upgrades.HostMachine
	}
	return ""
}

// WorkersStarted returns a channel that's closed once all top level workers
// have been started. This is provided for testing purposes.
func (a *MachineAgent) WorkersStarted() <-chan struct{} {
	return a.workersStarted

}

func (a *MachineAgent) Tag() string {
	return names.MachineTag(a.MachineId)
}

func (a *MachineAgent) createJujuRun(dataDir string) error {
	// TODO do not remove the symlink if it already points
	// to the right place.
	if err := os.Remove(jujuRun); err != nil && !os.IsNotExist(err) {
		return err
	}
	jujud := filepath.Join(dataDir, "tools", a.Tag(), "jujud")
	return os.Symlink(jujud, jujuRun)
}

func (a *MachineAgent) uninstallAgent(agentConfig agent.Config) error {
	var errors []error
	agentServiceName := agentConfig.Value(agent.AgentServiceName)
	if agentServiceName == "" {
		// For backwards compatibility, handle lack of AgentServiceName.
		agentServiceName = os.Getenv("UPSTART_JOB")
	}
	if agentServiceName != "" {
		if err := upstart.NewService(agentServiceName).Remove(); err != nil {
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
