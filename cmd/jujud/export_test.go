package main

import (
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type AgentCommand interface {
	cmd.Command
	JujuDir() string
	StateInfo() state.Info
}

func (a *agent) JujuDir() string {
	return a.jujuDir
}

func (a *agent) StateInfo() state.Info {
	return a.stateInfo
}
