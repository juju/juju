package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
)

type MachineFlags struct {
	agent *agent.MachineAgent
}

func NewMachineFlags() *MachineFlags {
	return &MachineFlags{&agent.MachineAgent{}}
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
	f.StringVar(&af.agent.Id, "machine-id", af.agent.Id, "id of the machine to run")
}

// ParsePositional checks that there are no unwanted arguments, and that any
// required flags have been set.
func (af *MachineFlags) ParsePositional(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	if af.agent.Id == "" {
		return requiredError("machine-id")
	}
	return nil
}
