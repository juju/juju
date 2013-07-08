// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"fmt"

        "launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.machiner")

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	st *state.State
	id string
        pinger *presence.Pinger
        machine *state.Machine
}

// NewMachiner returns a Machiner that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st *state.State, id string) worker.NotifyWorker {
	mr := &Machiner{st: st, id: id}
	return worker.NewNotifyWorker(mr)
}

func (mr *Machiner) String() string {
	return fmt.Sprintf("machiner %s", mr.id)
}

func (mr *Machiner) SetUp() (params.NotifyWatcher, error) {
	// Find which machine we're responsible for.
	m, err := mr.st.Machine(mr.id)
	if errors.IsNotFoundError(err) {
		return nil, worker.ErrTerminateAgent
	} else if err != nil {
		return nil, err
	}
        mr.machine = m

	// Announce our presence to the world.
	pinger, err := m.SetAgentAlive()
	if err != nil {
		return nil, err
	}
        // Now that this is added, TearDown will ensure it is cleaned up
        mr.pinger = pinger
	logger.Debugf("agent for machine %q is now alive", m)

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.StatusStarted, ""); err != nil {
		return nil, err
	}
	logger.Infof("machine %q started", m)

	w := m.Watch()
        return w, nil
}

func (mr *Machiner) Handle() error {
    if err := mr.machine.Refresh(); errors.IsNotFoundError(err) {
            return worker.ErrTerminateAgent
    } else if err != nil {
            return err
    }
    if mr.machine.Life() != state.Alive {
            logger.Debugf("machine %q is now %s", mr.machine, mr.machine.Life())
            if err := mr.machine.SetStatus(params.StatusStopped, ""); err != nil {
                    return err
            }
            // If the machine is Dying, it has no units,
            // and can be safely set to Dead.
            if err := mr.machine.EnsureDead(); err != nil {
                    return err
            }
            logger.Infof("machine %q shutting down", mr.machine)
            return worker.ErrTerminateAgent
    }
    return nil
}

func (mr *Machiner) TearDown() error {
    var err error
    if mr.pinger != nil {
        err = mr.pinger.Stop()
    }
    return err
}
