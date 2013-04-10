package machiner

import (
	"fmt"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/worker"
	"launchpad.net/tomb"
)

// Machiner is responsible for a machine agent's lifecycle.
type Machiner struct {
	tomb tomb.Tomb
	st   *state.State
	id   string
}

// NewMachiner returns a Machiner that will wait for the identified machine
// to become Dying and make it Dead; or until the machine becomes Dead by
// other means.
func NewMachiner(st *state.State, id string) *Machiner {
	mr := &Machiner{st: st, id: id}
	go func() {
		defer mr.tomb.Done()
		mr.tomb.Kill(mr.loop())
	}()
	return mr
}

func (mr *Machiner) String() string {
	return fmt.Sprintf("machiner %s", mr.id)
}

func (mr *Machiner) Stop() error {
	mr.tomb.Kill(nil)
	return mr.tomb.Wait()
}

func (mr *Machiner) Wait() error {
	return mr.tomb.Wait()
}

func (mr *Machiner) loop() error {
	// Find which machine we're responsible for.
	m, err := mr.st.Machine(mr.id)
	if state.IsNotFound(err) {
		return worker.ErrTerminateAgent
	} else if err != nil {
		return err
	}

	// Announce our presence to the world.
	pinger, err := m.SetAgentAlive()
	if err != nil {
		return err
	}
	log.Debugf("worker/machiner: agent for machine %q is now alive", m)
	defer watcher.Stop(pinger, &mr.tomb)

	// Mark the machine as started and log it.
	if err := m.SetStatus(params.MachineStarted, ""); err != nil {
		return err
	}
	log.Noticef("worker/machiner: machine %q started", m)

	w := m.Watch()
	defer watcher.Stop(w, &mr.tomb)
	for {
		select {
		case <-mr.tomb.Dying():
			return tomb.ErrDying
		case <-w.Changes():
			if err := m.Refresh(); state.IsNotFound(err) {
				return worker.ErrTerminateAgent
			} else if err != nil {
				return err
			}
			if m.Life() != state.Alive {
				log.Debugf("worker/machiner: machine %q is now %s", m, m.Life())
				if err := m.SetStatus(params.MachineStopped, ""); err != nil {
					return err
				}
				// If the machine is Dying, it has no units,
				// and can be safely set to Dead.
				if err := m.EnsureDead(); err != nil {
					return err
				}
				log.Noticef("worker/machiner: machine %q shutting down", m)
				return worker.ErrTerminateAgent
			}
		}
	}
	panic("unreachable")
}
