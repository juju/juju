package main

import "fmt"

// ProvisioningAgent is a cmd.Command responsible for running a provisioning agent.
type ProvisioningAgent struct {
	agentConf
}

func NewProvisioningAgent() *ProvisioningAgent {
	return &ProvisioningAgent{agentConf: agentConf{name: "provisioning"}}
}

// Run runs a provisioning agent.
func (a *ProvisioningAgent) Run() error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
