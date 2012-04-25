package main

import (
	"fmt"
	"launchpad.net/juju/go/cmd"
)

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	agentConf
}

func NewProvisioningAgent() *ProvisioningAgent {
	return &ProvisioningAgent{agentConf: agentConf{name: "provisioning"}}
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run(_ *cmd.Context) error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
