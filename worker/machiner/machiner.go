// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.machiner")

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	st      *machiner.State
	tag     string
	machine *machiner.Machine
}

// machineEnvironer is responsible for watching a machine agent's
// environment lifecycle, and terminating the agent if it becomes
// Dead.
type machineEnvironer struct {
	st         *machiner.State
	machineTag string
	environ    *machiner.Environment
}

// NewMachiner returns a Worker that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st *machiner.State, agentConfig agent.Config) worker.Worker {
	machiner := &Machiner{st: st, tag: agentConfig.Tag()}
	return worker.NewNotifyWorker(machiner)
}

func NewMachineEnvironer(st *machiner.State, agentConfig agent.Config) worker.Worker {
	machineEnvironer := &machineEnvironer{st: st, machineTag: agentConfig.Tag()}
	return worker.NewNotifyWorker(machineEnvironer)
}

func isNotFoundOrUnauthorized(err error) bool {
	return params.IsCodeNotFound(err) || params.IsCodeUnauthorized(err)
}

func (mr *Machiner) SetUp() (watcher.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.st.Machine(mr.tag)
	if isNotFoundOrUnauthorized(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, err
	}
	mr.machine = m

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.StatusStarted, "", nil); err != nil {
		return nil, fmt.Errorf("%s failed to set status started: %v", mr.tag, err)
	}
	logger.Infof("%q started", mr.tag)

	return m.Watch()
}

func (mr *Machiner) Handle() error {
	if err := mr.machine.Refresh(); isNotFoundOrUnauthorized(err) {
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}
	if mr.machine.Life() == params.Alive {
		return nil
	}
	logger.Debugf("%q is now %s", mr.tag, mr.machine.Life())
	if err := mr.machine.SetStatus(params.StatusStopped, "", nil); err != nil {
		return fmt.Errorf("%s failed to set status stopped: %v", mr.tag, err)
	}

	// If the machine is Dying, it has no units,
	// and can be safely set to Dead.
	if err := mr.machine.EnsureDead(); err != nil {
		return fmt.Errorf("%s failed to set machine to dead: %v", mr.tag, err)
	}
	return worker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}

func (me *machineEnvironer) SetUp() (watcher.NotifyWatcher, error) {
	env, err := me.st.Environment(me.machineTag)
	if isNotFoundOrUnauthorized(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, err
	}
	me.environ = env
	return env.Watch()
}

func (me *machineEnvironer) Handle() error {
	if err := me.environ.Refresh(); isNotFoundOrUnauthorized(err) {
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}
	if me.environ.Life() == params.Dead {
		logger.Infof("%q is dead, terminating agent", me.environ.Tag())
		return worker.ErrTerminateAgent
	}
	logger.Debugf("%q is now %s", me.environ.Tag(), me.environ.Life())
	return nil
}

func (me *machineEnvironer) TearDown() error {
	// Nothing to do here.
	return nil
}
