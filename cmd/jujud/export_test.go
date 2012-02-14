package main

import (
	"launchpad.net/juju/go/cmd"
)

type AgentCommand interface {
	cmd.Command
	JujuDir() string
	ZookeeperAddr() string
	SessionFile() string
}

func (c agentConf) JujuDir() string {
	return c.jujuDir
}

func (c agentConf) ZookeeperAddr() string {
	return c.zookeeperAddr
}

func (c agentConf) SessionFile() string {
	return c.sessionFile
}
