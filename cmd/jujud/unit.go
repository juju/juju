package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
)

type UnitFlags struct {
	agent *agent.UnitAgent
}

func NewUnitFlags() *UnitFlags {
	return &UnitFlags{&agent.UnitAgent{}}
}

// Name returns the agent's name.
func (af *UnitFlags) Name() string {
	return "unit"
}

// Agent returns the agent.
func (af *UnitFlags) Agent() agent.Agent {
	return af.agent
}

// InitFlagSet prepares a FlagSet.
func (af *UnitFlags) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&af.agent.Name, "unit-name", af.agent.Name, "name of the unit to run")
}

// ParsePositional checks that there are no unwanted arguments, and that any
// required flags have been set.
func (af *UnitFlags) ParsePositional(args []string) error {
	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	if af.agent.Name == "" {
		return requiredError("unit-name")
	}
	return nil
}
