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

func (c agent) JujuDir() string {
	return c.jujuDir
}

func (c agent) StateInfo() state.Info {
	return c.stateInfo
}
