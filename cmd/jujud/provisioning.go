package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	agent
}

func NewProvisioningAgent() *ProvisioningAgent {
	return &ProvisioningAgent{agent: agent{name: "provisioning"}}
}

// Init initializes the command for running.
func (a *ProvisioningAgent) Init(f *gnuflag.FlagSet, args []string) error {
	a.addFlags(f)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return a.checkArgs(f.Args())
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
