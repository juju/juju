package machiner

import (
	"fmt"
	"launchpad.net/juju-core/state"
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
	m, err := mr.st.Machine(mr.id)
	if state.IsNotFound(err) {
		return worker.ErrDead
	} else if err != nil {
		return err
	}
	w := m.Watch()
	defer watcher.Stop(w, &mr.tomb)
	for {
		select {
		case <-mr.tomb.Dying():
			return tomb.ErrDying
		case <-w.Changes():
			if err := m.Refresh(); state.IsNotFound(err) {
				return worker.ErrDead
			} else if err != nil {
				return err
			}
			if m.Life() != state.Alive {
				// If the machine is Dying, it has no units,
				// and can be safely set to Dead.
				if err := m.EnsureDead(); err != nil {
					return err
				}
				return worker.ErrDead
			}
		}
	}
	panic("unreachable")
}
