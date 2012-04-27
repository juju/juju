package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	agent
	MachineId int
}

func NewMachineAgent() *MachineAgent {
	return &MachineAgent{agent: agent{name: "machine"}}
}

// Init initializes the command for running.
func (a *MachineAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.addFlags(f)
	f.IntVar(&a.MachineId, "machine-id", -1, "id of the machine to run")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if a.MachineId < 0 {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	return a.checkArgs(f.Args())
}

// Run runs a machine agent.
func (a *MachineAgent) Run(_ *cmd.Context) error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
