package machiner

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// NewMachiner starts a machine agent running.
// The Machiner dies when it encounters an error.
func NewMachiner(st *state.State, machineId int) *Machiner {
	m := &Machiner{st: st}
	go m.loop(machineId)
	return m
}

// Machiner represents a running machine agent.
type Machiner struct {
	tomb tomb.Tomb
	st   *state.State
}

func (m *Machiner) loop(machineId int) {
	defer m.tomb.Done()

	machine, err := m.st.Machine(machineId)
	if err != nil {
		m.tomb.Kill(err)
		return
	}
	w := machine.WatchUnits()
	defer watcher.Stop(w, &m.tomb)

	// TODO read initial units, check if they're running
	// and restart them if not. Also track units so
	// that we don't deploy units that are already running.
	for {
		select {
		case <-m.tomb.Dying():
			return
		case change, ok := <-w.Changes():
			if !ok {
				m.tomb.Kill(watcher.MustErr(w))
				return
			}
			for _, u := range change.Removed {
				if u.IsPrincipal() {
					if err := container.Simple.Destroy(u); err != nil {
						log.Printf("cannot destroy unit %s: %v", u.Name(), err)
					}
				}
			}
			for _, u := range change.Added {
				if u.IsPrincipal() {
					if err := container.Simple.Deploy(u); err != nil {
						// TODO put unit into a queue to retry the deploy.
						log.Printf("cannot deploy unit %s: %v", u.Name(), err)
					}
				}
			}
		}
	}
}

// Wait waits until the Machiner has died, and returns the error encountered.
func (m *Machiner) Wait() error {
	return m.tomb.Wait()
}

// Stop terminates the Machiner and returns any error that it encountered.
func (m *Machiner) Stop() error {
	m.tomb.Kill(nil)
	return m.tomb.Wait()
}
