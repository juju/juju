package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
)

type ProvisioningFlags struct {
	agent *agent.ProvisioningAgent
}

func NewProvisioningFlags() *ProvisioningFlags {
	return &ProvisioningFlags{&agent.ProvisioningAgent{}}
}

// Name returns the agent's name.
func (af *ProvisioningFlags) Name() string {
	return "provisioning"
}

// Agent returns the agent.
func (af *ProvisioningFlags) Agent() agent.Agent {
	return af.agent
}

// InitFlagSet does nothing.
func (af *ProvisioningFlags) InitFlagSet(f *gnuflag.FlagSet) {}

// ParsePositional checks that there are no unwanted arguments.
func (af *ProvisioningFlags) ParsePositional(args []string) error {
	return cmd.CheckEmpty(args)
}
