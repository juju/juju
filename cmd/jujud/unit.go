package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/juju"
)

// UnitAgent is a cmd.Command responsible for running a unit agent.
type UnitAgent struct {
	agentConf
	UnitName string
}

func NewUnitAgent() *UnitAgent {
	return &UnitAgent{agentConf: agentConf{name: "unit"}}
}

// InitFlagSet prepares a FlagSet.
func (a *UnitAgent) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&a.UnitName, "unit-name", "", "name of the unit to run")
	a.agentConf.InitFlagSet(f)
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (a *UnitAgent) ParsePositional(args []string) error {
	if a.UnitName == "" {
		return requiredError("unit-name")
	}
	if !juju.ValidUnit.MatchString(a.UnitName) {
		return fmt.Errorf(`--unit-name option expects "<service>/<n>" argument`)
	}
	return a.agentConf.ParsePositional(args)
}

// Run runs a unit agent.
func (a *UnitAgent) Run(ctx *cmd.Context) error {
	return fmt.Errorf("UnitAgent.Run not implemented")
}
