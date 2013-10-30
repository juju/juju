// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/addressupdater"
	"launchpad.net/juju-core/worker/cleaner"
	"launchpad.net/juju-core/worker/deployer"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/localstorage"
	"launchpad.net/juju-core/worker/logger"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/juju-core/worker/minunitsworker"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/juju-core/worker/resumer"
	"launchpad.net/juju-core/worker/upgrader"
)

type workerRunner interface {
	worker.Worker
	StartWorker(id string, startFunc func() (worker.Worker, error)) error
	StopWorker(id string) error
}

var newRunner = func(isFatal func(error) bool, moreImportant func(e0, e1 error) bool) workerRunner {
	return worker.NewRunner(isFatal, moreImportant)
}

const bootstrapMachineId = "0"

var retryDelay = 3 * time.Second

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	cmd.CommandBase
	tomb      tomb.Tomb
	Conf      AgentConf
	MachineId string
	runner    workerRunner
}

// Info returns usage information for the command.
func (a *MachineAgent) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "machine",
		Purpose: "run a juju machine agent",
	}
}

func (a *MachineAgent) SetFlags(f *gnuflag.FlagSet) {
	a.Conf.addFlags(f)
	f.StringVar(&a.MachineId, "machine-id", "", "id of the machine to run")
}

// Init initializes the command for running.
func (a *MachineAgent) Init(args []string) error {
	if !names.IsMachine(a.MachineId) {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	if err := a.Conf.checkArgs(args); err != nil {
		return err
	}
	a.runner = newRunner(isFatal, moreImportant)
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
	cleanup := enableProfiling(a.Tag())
	defer cleanup()
	// file writer if one has been added, otherwise we will get duplicate
	// lines of all logging in the log file.
	loggo.RemoveWriter("logfile")
	defer a.tomb.Done()
	log.Infof("machine agent %v start", a.Tag())
	if err := a.Conf.read(a.Tag()); err != nil {
		return err
	}
	charm.CacheDir = filepath.Join(a.Conf.dataDir, "charmcache")

	// ensureStateWorker ensures that there is a worker that
	// connects to the state that runs within itself all the workers
	// that need a state connection. Unless we're bootstrapping, we
	// need to connect to the API server to find out if we need to
	// call this, so we make the APIWorker call it when necessary if
	// the machine requires it. Note that ensureStateWorker can be
	// called many times - StartWorker does nothing if there is
	// already a worker started with the given name.
	ensureStateWorker := func() {
		a.runner.StartWorker("state", a.StateWorker)
	}
	// We might be bootstrapping, and the API server is not
	// running yet. If so, make sure we run a state worker instead.
	if a.MachineId == bootstrapMachineId {
		// TODO(rog) When we have HA, we only want to do this
		// when we really are bootstrapping - once other
		// instances of the API server have been started, we
		// should follow the normal course of things and ignore
		// the fact that this was once the bootstrap machine.
		log.Infof("Starting StateWorker for machine-0")
		ensureStateWorker()
	}
	a.runner.StartWorker("api", func() (worker.Worker, error) {
		return a.APIWorker(ensureStateWorker)
	})
	err := a.runner.Wait()
	if err == worker.ErrTerminateAgent {
		err = a.uninstallAgent()
	}
	err = agentDone(err)
	a.tomb.Kill(err)
	return err
}

// APIWorker returns a Worker that connects to the API and starts any
// workers that need an API connection.
//
// If a state worker is necessary, APIWorker calls ensureStateWorker.
func (a *MachineAgent) APIWorker(ensureStateWorker func()) (worker.Worker, error) {
	agentConfig := a.Conf.config
	st, entity, err := openAPIState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	reportOpenedAPI(st)
	for _, job := range entity.Jobs() {
		if job.NeedsState() {
			ensureStateWorker()
			break
		}
	}
	runner := newRunner(connectionIsFatal(st), moreImportant)
	runner.StartWorker("machiner", func() (worker.Worker, error) {
		return machiner.NewMachiner(st.Machiner(), agentConfig), nil
	})
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return upgrader.NewUpgrader(st.Upgrader(), agentConfig), nil
	})
	runner.StartWorker("logger", func() (worker.Worker, error) {
		return logger.NewLogger(st.Logger(), agentConfig), nil
	})
	// At this stage, since we don't embed LXC containers, just start an lxc
	// provisioner task for non-lxc containers.  Since we have only LXC
	// containers and normal machines, this effectively means that we only
	// have an LXC provisioner when we have a normally provisioned machine
	// (through the environ-provisioner).  With the upcoming advent of KVM
	// containers, it is likely that we will want an LXC provisioner on a KVM
	// machine, and once we get nested LXC containers, we can remove this
	// check.
	//
	// TODO(dimitern) 2013-09-25 bug #1230289
	// Create jobs for container providers, rather than
	// using the provider and container type like this.
	providerType := agentConfig.Value(agent.ProviderType)
	if providerType != provider.Local && entity.ContainerType() != instance.LXC {
		workerName := fmt.Sprintf("%s-provisioner", provisioner.LXC)
		runner.StartWorker(workerName, func() (worker.Worker, error) {
			return provisioner.NewProvisioner(provisioner.LXC, st.Provisioner(), agentConfig), nil
		})
	}
	for _, job := range entity.Jobs() {
		switch job {
		case params.JobHostUnits:
			runner.StartWorker("deployer", func() (worker.Worker, error) {
				apiDeployer := st.Deployer()
				context := newDeployContext(apiDeployer, agentConfig)
				return deployer.NewDeployer(apiDeployer, context), nil
			})
		case params.JobManageEnviron:
			runner.StartWorker("environ-provisioner", func() (worker.Worker, error) {
				return provisioner.NewProvisioner(provisioner.ENVIRON, st.Provisioner(), agentConfig), nil
			})
			// TODO(dimitern): Add firewaller here, when using the API.
		case params.JobManageState:
			// Not yet implemented with the API.
		default:
			// TODO(dimitern): Once all workers moved over to using
			// the API, report "unknown job type" here.
		}
	}
	return newCloseWorker(runner, st), nil // Note: a worker.Runner is itself a worker.Worker.
}

// StateJobs returns a worker running all the workers that require
// a *state.State cofnnection.
func (a *MachineAgent) StateWorker() (worker.Worker, error) {
	agentConfig := a.Conf.config
	st, entity, err := openState(agentConfig, a)
	if err != nil {
		return nil, err
	}
	reportOpenedState(st)
	m := entity.(*state.Machine)

	runner := newRunner(connectionIsFatal(st), moreImportant)
	// Take advantage of special knowledge here in that we will only ever want
	// the storage provider on one machine, and that is the "bootstrap" node.
	providerType := agentConfig.Value(agent.ProviderType)
	if (providerType == provider.Local || providerType == provider.Null) && m.Id() == bootstrapMachineId {
		runner.StartWorker("local-storage", func() (worker.Worker, error) {
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
			// TODO(axw) 2013-09-24 bug #1229506
			// Make another job to enable the firewaller. Not all environments
			// are capable of managing ports centrally.
			runner.StartWorker("firewaller", func() (worker.Worker, error) {
				return firewaller.NewFirewaller(st), nil
			})
			runner.StartWorker("addressupdater", func() (worker.Worker, error) {
				return addressupdater.NewWorker(st), nil
			})
		case state.JobManageState:
			runner.StartWorker("apiserver", func() (worker.Worker, error) {
				// If the configuration does not have the required information,
				// it is currently not a recoverable error, so we kill the whole
				// agent, potentially enabling human intervention to fix
				// the agent's configuration file. In the future, we may retrieve
				// the state server certificate and key from the state, and
				// this should then change.
				port, cert, key := a.Conf.config.APIServerDetails()
				if len(cert) == 0 || len(key) == 0 {
					return nil, &fatalError{"configuration does not have state server cert/key"}
				}
				return apiserver.NewServer(st, fmt.Sprintf(":%d", port), cert, key)
			})
			runner.StartWorker("cleaner", func() (worker.Worker, error) {
				return cleaner.NewCleaner(st), nil
			})
			runner.StartWorker("resumer", func() (worker.Worker, error) {
				// The action of resumer is so subtle that it is not tested,
				// because we can't figure out how to do so without brutalising
				// the transaction log.
				return resumer.NewResumer(st), nil
			})
			runner.StartWorker("minunitsworker", func() (worker.Worker, error) {
				return minunitsworker.NewMinUnitsWorker(st), nil
			})
		default:
			log.Warningf("ignoring unknown job %q", job)
		}
	}
	return newCloseWorker(runner, st), nil
}

func (a *MachineAgent) Entity(st *state.State) (AgentState, error) {
	m, err := st.Machine(a.MachineId)
	if err != nil {
		return nil, err
	}
	// Check the machine nonce as provisioned matches the agent.Conf value.
	if !m.CheckProvisioned(a.Conf.config.Nonce()) {
		// The agent is running on a different machine to the one it
		// should be according to state. It must stop immediately.
		log.Errorf("running machine %v agent on inappropriate instance", m)
		return nil, worker.ErrTerminateAgent
	}
	return m, nil
}

func (a *MachineAgent) Tag() string {
	return names.MachineTag(a.MachineId)
}

func (m *MachineAgent) uninstallAgent() error {
	// TODO(axw) get this from agent config when it's available
	name := os.Getenv("UPSTART_JOB")
	if name != "" {
		return upstart.NewService(name).Remove()
	}
	return nil
}

// Below pieces are used for testing,to give us access to the *State opened
// by the agent, and allow us to trigger syncs without waiting 5s for them
// to happen automatically.

var stateReporter chan<- *state.State

func reportOpenedState(st *state.State) {
	select {
	case stateReporter <- st:
	default:
	}
}

func sendOpenedStates(dst chan<- *state.State) (undo func()) {
	var original chan<- *state.State
	original, stateReporter = stateReporter, dst
	return func() { stateReporter = original }
}

var apiReporter chan<- *api.State

func reportOpenedAPI(st *api.State) {
	select {
	case apiReporter <- st:
	default:
	}
}
func sendOpenedAPIs(dst chan<- *api.State) (undo func()) {
	var original chan<- *api.State
	original, apiReporter = apiReporter, dst
	return func() { apiReporter = original }
}
