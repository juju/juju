package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/agent"
	"launchpad.net/juju/go/cmd"
)

// AgentFlags is responsible for parsing agent-specific command line flags.
type AgentFlags interface {
	Name() string
	Agent() agent.Agent
	InitFlagSet(*gnuflag.FlagSet)
	ParsePositional([]string) error
}

// AgentCommand is responsible for parsing common agent command line flags; it
// delegates agent-specific behaviour to its contained AgentFlags.
type AgentCommand struct {
	agentFlags AgentFlags
	conf       *agent.AgentConf
}

func NewAgentCommand(agentFlags AgentFlags) *AgentCommand {
	return &AgentCommand{
		agentFlags: agentFlags,
		conf:       &agent.AgentConf{JujuDir: "/var/lib/juju"},
	}
}

// Info returns a description of the agent.
func (c *AgentCommand) Info() *cmd.Info {
	name := c.agentFlags.Name()
	return &cmd.Info{
		name,
		fmt.Sprintf("jujud %s [options]", name),
		fmt.Sprintf("run a juju %s agent", name),
		"",
	}
}

// InitFlagSet prepares a FlagSet.
func (c *AgentCommand) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.conf.JujuDir, "juju-directory", c.conf.JujuDir, "juju working directory")
	f.StringVar(&c.conf.Zookeeper, "z", c.conf.Zookeeper, "zookeeper servers to connect to")
	f.StringVar(&c.conf.Zookeeper, "zookeeper-servers", c.conf.Zookeeper, "")
	f.StringVar(&c.conf.SessionFile, "session-file", c.conf.SessionFile, "session id storage path")
	c.agentFlags.InitFlagSet(f)
}

// ParsePositional checks that required AgentConf flags have been set, and
// delegates to the AgentFlags for all other validation.
func (c *AgentCommand) ParsePositional(args []string) error {
	if c.conf.JujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.conf.Zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.conf.SessionFile == "" {
		return requiredError("session-file")
	}
	return c.agentFlags.ParsePositional(args)
}

// Run runs the agent.
func (c *AgentCommand) Run() error {
	return agent.Run(c.agentFlags.Agent(), c.conf)
}
