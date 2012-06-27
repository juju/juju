package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/tomb"
)

// simpleContainer allows tests to hook into the container
// deployment logic.
var simpleContainer = container.Simple

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	Conf      AgentConf
	MachineId int
}

// Info returns usage information for the command.
func (a *MachineAgent) Info() *cmd.Info {
	return &cmd.Info{"machine", "", "run a juju machine agent", ""}
}

// Init initializes the command for running.
func (a *MachineAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.Conf.addFlags(f)
	f.IntVar(&a.MachineId, "machine-id", -1, "id of the machine to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if a.MachineId < 0 {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	return a.Conf.checkArgs(f.Args())
}

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	// TODO reconnect when the machiner fails.
	m, err := NewMachiner(&a.Conf.StateInfo, a.MachineId)
	if err != nil {
		return err
	}
	go m.loop()
	return m.Wait()
}

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

	watcher := m.machine.WatchUnits()
	defer watcher.Stop()

	// TODO read initial units, check if they're running
	// and restart them if not. Also track units so
	// that we don't deploy units that are already running.
	for {
		select {
		case <-m.tomb.Dying():
			return
		case change, ok := <-watcher.Changes():
			if !ok {
				err := watcher.Stop()
				if err == nil {
					panic("watcher closed channel with no error")
				}
				m.tomb.Kill(err)
				return
			}
			for _, u := range change.Removed {
				if u.IsPrincipal() {
					if err := simpleContainer.Destroy(u); err != nil {
						log.Printf("cannot destroy unit %s: %v", u.Name(), err)
					}
				}
			}
			for _, u := range change.Added {
				if u.IsPrincipal() {
					if err := simpleContainer.Deploy(u); err != nil {
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
