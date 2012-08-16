package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/machiner"
	"launchpad.net/tomb"
	"time"
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	tomb      tomb.Tomb
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

// Stop stops the machine agent.
func (a *MachineAgent) Stop() error {
	a.tomb.Kill(nil)
	return a.tomb.Wait()
}

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	defer a.tomb.Done()
	for {
		err := a.runOnce()
		if err == nil {
			// We have been explicitly stopped.
			return nil
		}
		if ug, ok := err.(*UpgradedError); ok {
			log.Printf("upgrade to %v from %q", ug.Binary, ug.URL)
			// Return and let upstart deal with the restart.
			return nil
		}
		time.Sleep(retryDuration)
		log.Printf("restarting provisioner and firewaller after error: %v", err)
	}
	panic("unreachable")
}

func (a *MachineAgent) runOnce() error {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	defer st.Close()
	m, err := st.Machine(a.MachineId)
	if err != nil {
		return err
	}
	return runTasks(a.tomb.Dying(),
		machiner.NewMachiner(m),
		NewUpgrader("machine", m),
	)
}
