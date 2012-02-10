package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

// agentConf implements most of the cmd.Command interface, except for Run(),
// and is intended for embedding in types which implement juju agents, to
// help the agent types implement cmd.Command with minimal boilerplate.
type agentConf struct {
	name        string
	jujuDir     string // Defaults to "/var/lib/juju".
	zookeeper   string
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
	if c.jujuDir == "" {
		c.jujuDir = "/var/lib/juju"
	}
	f.StringVar(&c.jujuDir, "juju-directory", c.jujuDir, "juju working directory")
	f.StringVar(&c.zookeeper, "z", c.zookeeper, "zookeeper servers to connect to")
	f.StringVar(&c.zookeeper, "zookeeper-servers", c.zookeeper, "")
	f.StringVar(&c.sessionFile, "session-file", c.sessionFile, "session id storage path")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *agentConf) ParsePositional(args []string) error {
	if c.jujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.sessionFile == "" {
		return requiredError("session-file")
	}
	return cmd.CheckEmpty(args)
}
