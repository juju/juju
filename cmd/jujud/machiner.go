package main

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

// NewMachiner starts a machine agent running.
// The Machiner dies when it encounters an error.
func NewMachiner(info *state.Info, machineId int) (m *Machiner, err error) {
	m = new(Machiner)
	m.st, err = state.Open(info)
	if err != nil {
		return nil, err
	}
	m.machine, err = m.st.Machine(machineId)
	if err != nil {
		return nil, err
	}
	go m.loop()
	return m, nil
}

// Machiner represents a running machine agent.
type Machiner struct {
	tomb    tomb.Tomb
	st      *state.State
	machine *state.Machine
}

func (m *Machiner) loop() {
	defer m.tomb.Done()
	defer m.st.Close()
	w := m.machine.WatchUnits()
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
