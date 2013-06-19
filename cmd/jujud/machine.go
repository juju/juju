// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/tomb"
	"path/filepath"
	"time"
)

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
	log.Infof("machine agent start; tag %v", a.Tag())
	if err := a.Conf.read(a.Tag()); err != nil {
		return err
	}
	charm.CacheDir = filepath.Join(a.Conf.DataDir, "charmcache")
	defer a.tomb.Done()
	if a.MachineId == "0" {
		a.runner.StartWorker("state", a.StateWorker)
	}
	a.runner.StartWorker("api", a.APIWorker)
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
	params.JobServeAPI:      true,
}

func (a *MachineAgent) APIWorker() (worker.Worker, error) {
	log.Infof("opening api state with conf %#v", a.Conf.Conf)
	st, entity, err := openAPIState(a.Conf.Conf, a)
	if err != nil {
		log.Infof("open api failure: %v", err)
		return nil, err
	}
	log.Infof("open api success")
	m := entity.(*api.Machine)
	needsStateWorker := false
	for _, job := range m.Jobs() {
		needsStateWorker = needsStateWorker || stateJobs[job]
	}
	if needsStateWorker {
		// Start any workers that require a state connection.
		// Note the idempotency of StartWorker.
		a.runner.StartWorker("state", a.StateWorker)
	}
	runner := worker.NewRunner(allFatal, moreImportant)
	// No agents currently connect to the API, so just
	// return the runner running nothing.
	return newCloseWorker(runner, st), nil
}

// StateJobs returns a worker running all the workers that require
// a *state.State connection.
func (a *MachineAgent) StateWorker() (worker.Worker, error) {
	st, entity, err := openState(a.Conf.Conf, a)
	if err != nil {
		return nil, err
	}
	// TODO(rog) use more discriminating test for errors
	// rather than taking everything down indiscriminately.
	runner := worker.NewRunner(allFatal, moreImportant)
	runner.StartWorker("upgrader", func() (worker.Worker, error) {
		return NewUpgrader(st, m, a.Conf.DataDir), nil
	})
	runner.StartWorker("machiner", func() (worker.Worker, error) {
		return machiner.NewMachiner(st, m.Id()), nil
	})
	for _, job := range m.Jobs() {
		switch job {
 		case state.JobHostUnits:
			runner.StartWorker("deployer", func() (worker.Worker, error) {
				return newDeployer(st, m.WatchPrincipalUnits(), a.Conf.DataDir), nil
			})
		case state.JobManageEnviron:
			runner.StartWorker("provisioner", func() (worker.Worker, error) {
				return provisioner.NewProvisioner(st, a.MachineId), nil
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
	m, err := st.Machine(a.MachineId)
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
