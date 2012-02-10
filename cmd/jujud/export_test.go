package main

import (
	"launchpad.net/juju/go/cmd"
)

type AgentCommand interface {
	cmd.Command
	JujuDir() string
	Zookeeper() string
	SessionFile() string
}

func (c *agentConf) JujuDir() string {
	return c.jujuDir
}

func (c *agentConf) Zookeeper() string {
	return c.zookeeper
}

func (c *agentConf) SessionFile() string {
	return c.sessionFile
}
