package machiner

import (
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
)

var deploy = container.Deploy
var destroy = container.Destroy

// Machiner represents a running machine agent.
type Machiner struct {
	tomb      tomb.Tomb
	machine *state.Machine
	cfg container.Config
	stateInfo *state.Info
	tools *state.Tools
}

// NewMachiner starts a machine agent running that deploys agents in the
// given directory.  The Machiner dies when it encounters an error.
func NewMachiner(machine *state.Machine, info *state.Info, varDir string) *Machiner {
	tools, err := environs.ReadTools(varDir, version.Current)
	if err != nil {
		tools = &state.Tools{Binary: version.Current}
	}
	m := &Machiner{
		machine: machine,
		stateInfo: info,
		tools: tools,
		cfg: container.Config{
			VarDir: varDir,
		},
	}
	go m.loop()
	return m
}

func (m *Machiner) loop() {
	defer m.tomb.Done()
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
					if err := destroy(m.cfg, u); err != nil {
						log.Printf("cannot destroy unit %s: %v", u.Name(), err)
					}
				}
			}
			for _, u := range change.Added {
				if u.IsPrincipal() {
					if err := deploy(m.cfg, u, m.stateInfo, m.tools); err != nil {
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
