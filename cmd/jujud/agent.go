package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

// agentConf implements most of the cmd.Command interface, except for Run(),
// and is intended for embedding in types which implement juju agents, to
// help the agent types implement cmd.Command with minimal boilerplate.
type agentConf struct {
	name        string
	jujuDir     string // Defaults to "/var/lib/juju".
	stateInfo   state.Info
	sessionFile string
}

// Info returns a decription of the command.
func (c *agentConf) Info() *cmd.Info {
	return &cmd.Info{
		c.name, "[options]",
		fmt.Sprintf("run a juju %s agent", c.name),
		"",
		true,
	}
}

// InitFlagSet prepares a FlagSet.
func (c *agentConf) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.jujuDir, "juju-directory", "/var/lib/juju", "juju working directory")
	stateInfoVar(f, &c.stateInfo, "zookeeper-servers", nil, "zookeeper servers to connect to")
	f.StringVar(&c.sessionFile, "session-file", "", "session id storage path")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *agentConf) ParsePositional(args []string) error {
	if c.jujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.stateInfo.Addrs == nil {
		return requiredError("zookeeper-servers")
	}
	if c.sessionFile == "" {
		return requiredError("session-file")
	}
	return cmd.CheckEmpty(args)
}
