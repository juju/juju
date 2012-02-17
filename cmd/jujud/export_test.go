package main

import (
	"launchpad.net/juju/go/cmd"
)

type AgentCommand interface {
	cmd.Command
	JujuDir() string
	SessionFile() string
	ZookeeperAddrs() []string
}

func (c agentConf) JujuDir() string {
	return c.jujuDir
}

func (c agentConf) SessionFile() string {
	return c.sessionFile
}

func (c agentConf) ZookeeperAddrs() []string {
	return c.zookeeperAddrs
}
