// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
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

// NewMachiner returns a Worker that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st *machiner.State, tag string) worker.Worker {
	mr := &Machiner{st: st, tag: tag}
	return worker.NewNotifyWorker(mr)
}

func isNotFoundOrUnauthorized(err error) bool {
	return errors.IsNotFoundError(err) || params.ErrCode(err) == params.CodeUnauthorized
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
	if err := m.SetStatus(params.StatusStarted, ""); err != nil {
		logger.Errorf("%s failed to set status started: %v", mr, err)
		return nil, err
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
	if err := mr.machine.SetStatus(params.StatusStopped, ""); err != nil {
		logger.Errorf("%s failed to set status stopped: %v", mr, err)
		return err
	}

	// If the machine is Dying, it has no units,
	// and can be safely set to Dead.
	if err := mr.machine.EnsureDead(); err != nil {
		logger.Errorf("%s falied to set machine to dead: %v", mr, err)
		return err
	}
	return worker.ErrTerminateAgent
}

func (mr *Machiner) TearDown() error {
	// Nothing to do here.
	return nil
}
