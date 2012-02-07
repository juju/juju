package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
	"strconv"
)

type MachineFlags struct {
	machineId string
	agent     *agent.Machine
}

func NewMachineFlags() *MachineFlags {
	return &MachineFlags{agent: &agent.Machine{}}
}

// Name returns the agent's name.
func (af *MachineFlags) Name() string {
	return "machine"
}

// Agent returns the agent.
func (af *MachineFlags) Agent() agent.Agent {
	return af.agent
}

// InitFlagSet prepares a FlagSet.
func (af *MachineFlags) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&af.machineId, "machine-id", af.machineId, "id of the machine to run")
}

// ParsePositional checks that there are no unwanted arguments, and that any
// required flags have been set.
func (af *MachineFlags) ParsePositional(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	if af.machineId == "" {
		return requiredError("machine-id")
	}
	id, err := strconv.ParseUint(af.machineId, 10, 0)
	if err != nil {
		return fmt.Errorf("--machine-id option expects a non-negative integer")
	}
	af.agent.Id = uint(id)
	return nil
}
