package main

import (
	"launchpad.net/juju/go/cmd"
)

type AgentCommand interface {
	cmd.Command
	Conf() *AgentConf
}

func (c *UnitCommand) Conf() *AgentConf {
	return c.conf
}
func (c *MachineCommand) Conf() *AgentConf {
	return c.conf
}
func (c *ProvisioningCommand) Conf() *AgentConf {
	return c.conf
}
