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
	defer log.Printf("machine agent exiting")
	defer a.tomb.Done()
	for a.tomb.Err() == tomb.ErrStillAlive {
		log.Printf("machine agent starting")
		err := a.runOnce()
		if ug, ok := err.(*UpgradeReadyError); ok {
			if err = ug.Upgrade(); err == nil {
				// Return and let upstart deal with the restart.
				return ug
			}
		}
		if err == machiner.ErrDead {
			return nil
		}
		select {
		case <-a.tomb.Dying():
			a.tomb.Kill(err)
		case <-time.After(retryDelay):
			log.Printf("restarting machiner after error: %v", err)
		}
	}
	return a.tomb.Err()
}

func (a *MachineAgent) runOnce() error {
	st, err := state.Open(&a.Conf.StateInfo)
	if err != nil {
		return err
	}
	defer st.Close()
	m, err := st.Machine(a.MachineId)
	if state.IsNotFound(err) || err == nil && m.Life() == state.Dead {
		return machiner.ErrDead
	}
	if err != nil {
		return err
	}
	return runTasks(a.tomb.Dying(),
		machiner.NewMachiner(m, &a.Conf.StateInfo, a.Conf.DataDir),
		NewUpgrader(st, m, a.Conf.DataDir),
	)
}
