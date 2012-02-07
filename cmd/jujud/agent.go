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
	JujuDir     string
	Zookeeper   string
	SessionFile string
	agentFlags  AgentFlags
}

func NewAgentCommand(agentFlags AgentFlags) *AgentCommand {
	return &AgentCommand{
		agentFlags: agentFlags,
		JujuDir:    "/var/lib/juju",
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
	f.StringVar(&c.JujuDir, "juju-directory", c.JujuDir, "juju working directory")
	f.StringVar(&c.Zookeeper, "z", c.Zookeeper, "zookeeper servers to connect to")
	f.StringVar(&c.Zookeeper, "zookeeper-servers", c.Zookeeper, "")
	f.StringVar(&c.SessionFile, "session-file", c.SessionFile, "session id storage path")
	c.agentFlags.InitFlagSet(f)
}

// ParsePositional checks that required flags have been set, and delegates to
// the AgentFlags for all other validation.
func (c *AgentCommand) ParsePositional(args []string) error {
	if c.JujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.Zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.SessionFile == "" {
		return requiredError("session-file")
	}
	return c.agentFlags.ParsePositional(args)
}

// Run runs the agent.
func (c *AgentCommand) Run() error {
	// TODO (re)connect once new state.Open is available
	// (note, Zookeeper will likely need to become some sort of StateInfo)
	// state, err := state.Open(conf.Zookeeper, conf.SessionFile)
	// if err != nil {
	//     return err
	// }
	// defer state.Close()
	// return c.agentFlags.Agent().Run(state, conf.JujuDir)
	return fmt.Errorf("agent.Run not implemented")
}
