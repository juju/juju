package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type ProvisioningCommand struct {
	conf *AgentConf
}

func NewProvisioningCommand() *ProvisioningCommand {
	return &ProvisioningCommand{conf: NewAgentConf()}
}

// Info returns a decription of the command.
func (c *ProvisioningCommand) Info() *cmd.Info {
	return &cmd.Info{"provisioning", "[options]", "run a juju provisioning agent", "", true}
}

// InitFlagSet prepares a FlagSet.
func (c *ProvisioningCommand) InitFlagSet(f *gnuflag.FlagSet) {
	c.conf.InitFlagSet(f)
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *ProvisioningCommand) ParsePositional(args []string) error {
	if err := c.conf.Validate(); err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

// Run runs a provisioning agent.
func (c *ProvisioningCommand) Run() error {
	return StartAgent(c.conf, &ProvisioningAgent{})
}

// ProvisioningAgent is responsible for launching new machines.
type ProvisioningAgent struct {
}

// Run runs the agent.
func (a *ProvisioningAgent) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("ProvisioningAgent.Run not implemented")
}
