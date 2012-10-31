package machiner

import (
	"fmt"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
	"launchpad.net/tomb"
)

// Machiner represents a running machine agent.
type Machiner struct {
	tomb           tomb.Tomb
	machine        *state.Machine
	localContainer container.Container
	stateInfo      *state.Info
	tools          *state.Tools
}

// NewMachiner starts a machine agent running that
// deploys agents in the given directory.
// The Machiner dies when it encounters an error.
func NewMachiner(machine *state.Machine, info *state.Info, dataDir string) *Machiner {
	cont := &container.Simple{DataDir: dataDir}
	return newMachiner(machine, info, dataDir, cont)
}

func newMachiner(machine *state.Machine, info *state.Info, dataDir string, cont container.Container) *Machiner {
	tools, err := environs.ReadTools(dataDir, version.Current)
	if err != nil {
		tools = &state.Tools{Binary: version.Current}
	}
	m := &Machiner{
		machine:        machine,
		stateInfo:      info,
		tools:          tools,
		localContainer: cont,
	}
	go m.loop()
	return m
}

func (m *Machiner) loop() {
	defer m.tomb.Done()
	w := m.machine.WatchPrincipalUnits()
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
					if err := m.localContainer.Destroy(u); err != nil {
						log.Printf("worker/machiner: cannot destroy unit %s: %v", u.Name(), err)
					}
				}
			}
			for _, u := range change.Added {
				if u.IsPrincipal() {
					if err := m.localContainer.Deploy(u, m.stateInfo, m.tools); err != nil {
						// TODO put unit into a queue to retry the deploy.
						log.Printf("worker/machiner: cannot deploy unit %s: %v", u.Name(), err)
					}
				}
			}
		}
	}
}

func (a *Machiner) String() string {
	return fmt.Sprintf("machiner for machine %d", a.machine.Id())
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
