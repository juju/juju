package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/juju/cmd"
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/tomb"
	"launchpad.net/juju-core/juju/container"
	"launchpad.net/juju-core/juju/state"
)

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
	// TODO reconnect when the agent fails.
	m, err := NewMachiner(&a.Conf.StateInfo, a.MachineId)
	if err != nil {
		return err
	}
	go m.loop()
	return m.Wait()
}

func NewMachiner(info *state.Info, machineId int) (*Machiner, error) {
	m := new(Machiner)
	var err error
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

type Machiner struct {
	tomb tomb.Tomb
	st *state.State
	machine *state.Machine
}

func (m *Machiner) loop() {
	defer m.tomb.Done()
	defer m.st.Close()

	watcher := m.machine.WatchUnits()
	// TODO read initial units, check if they're running
	// and restart them if not.
	for {
		select {
		case <-m.tomb.Dying():
			return
		case change := <-watcher.Changes():
			for _, u := range change.Deleted {
				if u.IsPrincipal() {
					if err := container.Simple(u).Destroy(); err != nil {
						log.Printf("cannot destroy unit %s: %v", u.Name(), err)
					}
				}
			}
			for _, u := range change.Added {
				if u.IsPrincipal() {
					if err := container.Simple(u).Deploy(); err != nil {
						log.Printf("cannot deploy unit %s: %v", u.Name(), err)
					}
				}
			}
		}
	}
}

func (m *Machiner) Wait() error {
	return m.tomb.Wait()
}

func (m *Machiner) Stop() error {
	m.tomb.Kill(nil)
	return m.tomb.Wait()
}
