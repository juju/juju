package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"strconv"
)

// MachineAgent is a cmd.Command responsible for running a machine agent.
type MachineAgent struct {
	agentConf
	machineId string
	MachineId uint
}

func NewMachineAgent() *MachineAgent {
	return &MachineAgent{agentConf: agentConf{name: "machine"}}
}

// InitFlagSet prepares a FlagSet.
func (a *MachineAgent) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&a.machineId, "machine-id", a.machineId, "id of the machine to run")
	a.agentConf.InitFlagSet(f)
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (a *MachineAgent) ParsePositional(args []string) error {
	if a.machineId == "" {
		return requiredError("machine-id")
	}
	id, err := strconv.ParseUint(a.machineId, 10, 0)
	if err != nil {
		return fmt.Errorf("--machine-id option expects a non-negative integer")
	}
	a.MachineId = uint(id)
	return a.agentConf.ParsePositional(args)
}

// Run runs a machine agent.
func (a *MachineAgent) Run() error {
	// TODO connect to state once Open interface settles down
	// state, err := state.Open(a.zookeeperAddr, a.sessionFile)
	// ...
	return fmt.Errorf("MachineAgent.Run not implemented")
}
