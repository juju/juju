package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/worker/machiner"
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
	// TODO reconnect when the machiner fails.
	m, err := machiner.NewMachiner(&a.Conf.StateInfo, a.MachineId)
	if err != nil {
		return err
	}
	return m.Wait()
}
