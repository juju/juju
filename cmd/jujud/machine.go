// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/gnuflag"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	localstorage "launchpad.net/juju-core/environs/local/storage"
	"launchpad.net/juju-core/environs/provider"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/machineagent"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/cleaner"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/juju-core/worker/resumer"
)

const bootstrapMachineId = "0"

var retryDelay = 3 * time.Second

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	cmd.CommandBase
	tomb      tomb.Tomb
	Conf      AgentConf
	MachineId string
	runner    *worker.Runner
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
	if !state.IsMachineId(a.MachineId) {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	if err := a.Conf.checkArgs(args); err != nil {
		return err
	}
	a.runner = worker.NewRunner(isFatal, moreImportant)
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
	defer a.tomb.Done()
	log.Infof("machine agent %v start", a.Tag())
	if err := a.Conf.read(a.Tag()); err != nil {
		return err
	}
	if err := EnsureWeHaveLXC(a.Conf.DataDir, a.Tag()); err != nil {
		log.Errorf("we were unable to install the lxc package, unable to continue: %v", err)
		return err
	}
	charm.CacheDir = filepath.Join(a.Conf.DataDir, "charmcache")

	// ensureStateWorker ensures that there is a worker that
	// connects to the state that runs within itself all the workers
	// that need a state connection Unless we're bootstrapping, we
	// need to connect to the API server to find out if we need to
	// call this, so we make the APIWorker call it when necessary if
	// the machine requires it.  Note that ensureStateWorker can be
	// called many times - StartWorker does nothing if there is
	// already a worker started with the given name.
	ensureStateWorker := func() {
		a.runner.StartWorker("state", func() (worker.Worker, error) {
			// TODO(rog) go1.1: use method expression
			return a.StateWorker()
		})
	}
	if a.MachineId == bootstrapMachineId {
		// If we're bootstrapping, we don't have an API
		// server to connect to, so start the state worker regardless.

		// TODO(rog) When we have HA, we only want to do this
		// when we really are bootstrapping - once other
		// instances of the API server have been started, we
		// should follow the normal course of things and ignore
		// the fact that this was once the bootstrap machine.
		log.Infof("Starting StateWorker for machine-0")
		ensureStateWorker()
	}
	a.runner.StartWorker("api", func() (worker.Worker, error) {
		// TODO(rog) go1.1: use method expression
		return a.APIWorker(ensureStateWorker)
	})
	err := agentDone(a.runner.Wait())
	a.tomb.Kill(err)
	return err
}

func allFatal(error) bool {
	return true
}

var stateJobs = map[params.MachineJob]bool{
	params.JobHostUnits:     true,
	params.JobManageEnviron: true,
	params.JobManageState:   true,
}

// APIWorker returns a Worker that connects to the API and starts any
// workers that need an API connection.
//
// If a state worker is necessary, APIWorker calls ensureStateWorker.
func (a *MachineAgent) APIWorker(ensureStateWorker func()) (worker.Worker, error) {
	st, entity, err := openAPIState(a.Conf.Conf, a)
	if err != nil {
		// There was an error connecting to the API,
		// https://launchpad.net/bugs/1199915 means that we may just
		// not have an API password set. So force a state connection at
		// this point.
		// TODO(jam): Once we can reliably trust that we have API
		//            passwords set, and we no longer need state
		//            connections (and possibly agents will be blocked
		//            from connecting directly to state) we can remove
		//            this. Currently needed because 1.10 does not set
		//            the API password and 1.11 requires it
		ensureStateWorker()
		return nil, err
	}
	m := entity.(*machineagent.Machine)
	needsStateWorker := false
	for _, job := range m.Jobs() {
		needsStateWorker = needsStateWorker || stateJobs[job]
	}
	if needsStateWorker {
		ensureStateWorker()
	}
	runner := worker.NewRunner(allFatal, moreImportant)
	// Only the machiner currently connects to the API.
	// Add other workers here as they are ready.
	runner.StartWorker("machiner", func() (worker.Worker, error) {
		return machiner.NewMachiner(st.Machiner(), a.Tag()), nil
	})
	return newCloseWorker(runner, st), nil // Note: a worker.Runner is itself a worker.Worker.
}

// StateJobs returns a worker running all the workers that require
// a *state.State connection.
func (a *MachineAgent) StateWorker() (worker.Worker, error) {
	st, entity, err := openState(a.Conf.Conf, a)
	if err != nil {
		return nil, err
	}
	// If this fails, other bits will fail, so we just log the error, and
	// let the other failures actually restart runners
	if err := EnsureAPIInfo(a.Conf.Conf, st, entity); err != nil {
		log.Warningf("failed to EnsureAPIInfo: %v", err)
	}
	reportOpenedState(st)
	m := entity.(*state.Machine)
	// TODO(rog) use more discriminating test for errors
	// rather than taking everything down indiscriminately.
	dataDir := a.Conf.DataDir
	runner := worker.NewRunner(allFatal, moreImportant)
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		// TODO(rog) use id instead of *Machine (or introduce Clone method)
		return NewUpgrader(st, m, dataDir), nil
	})
	// At this stage, since we don't embed lxc containers, just start an lxc
	// provisioner task for non-lxc containers.  Since we have only LXC
	// containers and normal machines, this effectively means that we only
	// have an LXC provisioner when we have a normally provisioned machine
	// (through the environ-provisioner).  With the upcoming advent of KVM
	// containers, it is likely that we will want an LXC provisioner on a KVM
	// machine, and once we get nested LXC containers, we can remove this
	// check.
	providerType := os.Getenv("JUJU_PROVIDER_TYPE")
	if providerType != provider.Local && m.ContainerType() != instance.LXC {
		workerName := fmt.Sprintf("%s-provisioner", provisioner.LXC)
		runner.StartWorker(workerName, func() (worker.Worker, error) {
			return provisioner.NewProvisioner(provisioner.LXC, st, a.MachineId, dataDir), nil
		})
	}
	// Take advantage of special knowledge here in that we will only ever want
	// the storage provider on one machine, and that is the "bootstrap" node.
	if providerType == provider.Local && m.Id() == bootstrapMachineId {
		runner.StartWorker("local-storage", func() (worker.Worker, error) {
			return localstorage.NewWorker(), nil
		})
	}
	for _, job := range m.Jobs() {
		switch job {
		case state.JobHostUnits:
			runner.StartWorker("deployer", func() (worker.Worker, error) {
				return newDeployer(st, m.Id(), dataDir), nil
			})
		case state.JobManageEnviron:
			runner.StartWorker("environ-provisioner", func() (worker.Worker, error) {
				return provisioner.NewProvisioner(provisioner.ENVIRON, st, a.MachineId, dataDir), nil
			})
			runner.StartWorker("firewaller", func() (worker.Worker, error) {
				return firewaller.NewFirewaller(st), nil
			})
		case state.JobManageState:
			runner.StartWorker("apiserver", func() (worker.Worker, error) {
				// If the configuration does not have the required information,
				// it is currently not a recoverable error, so we kill the whole
				// agent, potentially enabling human intervention to fix
				// the agent's configuration file. In the future, we may retrieve
				// the state server certificate and key from the state, and
				// this should then change.
				if len(a.Conf.StateServerCert) == 0 || len(a.Conf.StateServerKey) == 0 {
					return nil, &fatalError{"configuration does not have state server cert/key"}
				}
				return apiserver.NewServer(st, fmt.Sprintf(":%d", a.Conf.APIPort), a.Conf.StateServerCert, a.Conf.StateServerKey)
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
	if !m.CheckProvisioned(a.Conf.MachineNonce) {
		// The agent is running on a different machine to the one it
		// should be according to state. It must stop immediately.
		log.Errorf("running machine %v agent on inappropriate instance", m)
		return nil, worker.ErrTerminateAgent
	}
	return m, nil
}

func (a *MachineAgent) APIEntity(st *api.State) (AgentAPIState, error) {
	m, err := st.MachineAgent().Machine(a.Tag())
	if err != nil {
		return nil, err
	}
	// TODO(rog) move the CheckProvisioned test into
	// this method when it's implemented in the API
	return m, nil
}

func (a *MachineAgent) Tag() string {
	return state.MachineTag(a.MachineId)
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
