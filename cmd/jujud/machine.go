package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	agentConf
	MachineId int
}

func NewMachineAgent() *MachineAgent {
	return &MachineAgent{agentConf: agentConf{name: "machine"}}
}

// InitFlagSet prepares a FlagSet.
func (a *MachineAgent) InitFlagSet(f *gnuflag.FlagSet) {
	f.IntVar(&a.MachineId, "machine-id", -1, "id of the machine to run")
	a.agentConf.InitFlagSet(f)
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (a *MachineAgent) ParsePositional(args []string) error {
	if a.MachineId < 0 {
		return fmt.Errorf("--machine-id option must be set, and expects a non-negative integer")
	}
	return a.agentConf.ParsePositional(args)
}

// Run runs a machine agent.
func (a *MachineAgent) Run(ctx *cmd.Context) error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
