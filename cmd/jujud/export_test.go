package main

import (
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type AgentCommand interface {
	cmd.Command
	JujuDir() string
	SessionFile() string
	StateInfo() state.Info
}

func (c agentConf) JujuDir() string {
	return c.jujuDir
}

func (c agentConf) SessionFile() string {
	return c.sessionFile
}

func (c agentConf) StateInfo() state.Info {
	return c.stateInfo
}
