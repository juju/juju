// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujud/tasks"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/firewaller"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/juju-core/worker/provisioner"
	"launchpad.net/tomb"
	"path/filepath"
	"sync"
	"time"
)

var retryDelay = 3 * time.Second

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	cmd.CommandBase
	tomb      tomb.Tomb
	Conf      AgentConf
	MachineId string
	runner    *tasks.Runner
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
	a.runner = tasks.NewRunner(isFatal, moreImportant)
	return nil
}

// Stop stops the machine agent.
func (a *MachineAgent) Stop() error {
	return a.runner.Stop()
}

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	if err := a.Conf.read(a.Tag()); err != nil {
		return err
	}
	charm.CacheDir = filepath.Join(a.Conf.DataDir, "charmcache")
	defer a.tomb.Done()
	if a.MachineId == "0" {
		a.runner.StartTask("state", a.StateTask)
	}
	a.runner.StartTask("api", a.APITask)
	return agentDone(a.runner.Wait())
}

func allFatal(error) bool {
	return true
}

func (a *MachineAgent) RunOnce(st *state.State, e AgentState) error {
	return fmt.Errorf("remove me!")
}

var stateJobs = map[state.MachineJob]bool{
	state.JobHostUnits:     true,
	state.JobManageEnviron: true,
	state.JobServeAPI:      true,
}

func (a *MachineAgent) APITask() (tasks.Task, error) {
	st, entity, err := openAPIState(a.Conf.Conf, a)
	if err != nil {
		return nil, err
	}
	m := entity.(*api.Machine)
	needsStateTask := false
	for _, job := range m.Jobs() {
		needsStateTask = needsStateTask || stateJobs[job]
	}
	if needsStateTask {
		// Start any tasks that require a state connection.
		// Note the idempotency of StartTask.
		a.runner.StartTask("state", a.StateTask)
	}
	runner := tasks.NewRunner(allFatal, moreImportant)
	// No agents currently connect to the API, so just
	// return the runner running nothing.
	return runner, nil
}

// StateJobs returns a task running all the workers that require
// a *state.State connection.
func (a *MachineAgent) StateTask() (tasks.Task, error) {
	st, _, err := a.Conf.OpenState()
	if err != nil {
		return nil, err
	}
	m, err := st.Machine(a.MachineId)
	if err != nil {
		return nil, err
	}
	// TODO(rog) use more discriminating test for errors
	// rather than taking everything down indiscriminately.
	runner := tasks.NewRunner(allFatal, moreImportant)
	runner.StartTask("upgrader", func() (tasks.Task, error) {
		return NewUpgrader(st, m, a.Conf.DataDir), nil
	})
	runner.StartTask("machiner", func() (tasks.Task, error) {
		return machiner.NewMachiner(st, m.Id()), nil
	})
	for _, job := range m.Jobs() {
		switch job {
 		case state.JobHostUnits:
			runner.StartTask("deployer", func() (tasks.Task, error) {
				return newDeployer(st, m.WatchPrincipalUnits(), a.Conf.DataDir), nil
			})
		case state.JobManageEnviron:
			runner.StartTask("provisioner", func() (tasks.Task, error) {
				return provisioner.NewProvisioner(st, a.MachineId), nil
			})
			runner.StartTask("firewaller", func() (tasks.Task, error) {
				return firewaller.NewFirewaller(st), nil
			})
		case state.JobServeAPI:
			runner.StartTask("apiserver", func() (tasks.Task, error) {
				// If the configuration does not have the required information,
				// it is currently not a recoverable error, so we kill the whole
				// agent, potentially enabling human intervention to fix
				// the agent's configuration file. In the future, we may retrieve
				// the state server certificate and key from the state, and
				// this should then change.
				if len(conf.StateServerCert) == 0 || len(conf.StateServerKey) == 0 {
					return nil, &fatalError{"configuration does not have state server cert/key"}
				}
				return apiserver.NewServer(st, fmt.Sprintf(":%d", conf.APIPort), conf.StateServerCert, conf.StateServerKey)
			})
		default:
			log.Warningf("ignoring unknown job %q", j)
		}
	}
	return newCloserTask(runner, st)
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

func (a *MachineAgent) Tag() string {
	return state.MachineTag(a.MachineId)
}
