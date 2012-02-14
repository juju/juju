package main

import (
	"fmt"
	"launchpad.net/gnuflag"
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	agentConf
	MachineId int
}

func NewMachineAgent() *MachineAgent {
	return &MachineAgent{agentConf: agentConf{name: "machine"}, MachineId: -1}
}

// InitFlagSet prepares a FlagSet.
func (a *MachineAgent) InitFlagSet(f *gnuflag.FlagSet) {
	f.IntVar(&a.MachineId, "machine-id", a.MachineId, "id of the machine to run")
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
func (a *MachineAgent) Run() error {
	// TODO connect to state once Open interface settles down
	// state, err := state.Open(a.zookeeperAddr, a.sessionFile)
	// ...
	return fmt.Errorf("MachineAgent.Run not implemented")
}
