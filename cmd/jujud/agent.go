package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/state"
)

// Agent must be implemented by every agent to be used with AgentConf.
type Agent interface {
	Run(state *state.State, jujuDir string) error
}

// AgentConf is responsible for parsing command-line arguments common to every
// agent, and for running an Agent in the environment defined by those args.
type AgentConf struct {
	JujuDir     string
	Zookeeper   string
	SessionFile string
}

func NewAgentConf() *AgentConf {
	return &AgentConf{JujuDir: "/var/lib/juju"}
}

// InitFlagSet prepares a FlagSet.
func (c *AgentConf) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.JujuDir, "juju-directory", c.JujuDir, "juju working directory")
	f.StringVar(&c.Zookeeper, "z", c.Zookeeper, "zookeeper servers to connect to")
	f.StringVar(&c.Zookeeper, "zookeeper-servers", c.Zookeeper, "")
	f.StringVar(&c.SessionFile, "session-file", c.SessionFile, "session id storage path")
}

// Validate returns an error if any fields are unset.
func (c *AgentConf) Validate() error {
	if c.JujuDir == "" {
		return requiredError("juju-directory")
	}
	if c.Zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.SessionFile == "" {
		return requiredError("session-file")
	}
	return nil
}

// Run runs the Agent in the environment specified in the AgentConf.
func (c *AgentConf) Run(a Agent) error {
	// TODO (re)connect once new state.Open is available
	// (note, Zookeeper will likely need to become some sort of StateInfo)
	// state, err := state.Open(c.Zookeeper, c.SessionFile)
	// if err != nil {
	//     return err
	// }
	// defer state.Close()
	// return a.Run(state, conf.JujuDir)
	return fmt.Errorf("AgentConf.Run not implemented")
}
