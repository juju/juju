// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
agent contains jujud's machine agent.
*/
package agent

import (
	"sync"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

type AgentConf interface {
	// AddFlags injects common agent flags into f.
	AddFlags(f *gnuflag.FlagSet)
	// CheckArgs reports whether the given args are valid for this agent.
	CheckArgs(args []string) error
	// ReadConfig reads the agent's config from its config file.
	ReadConfig(tag string) error
	// ChangeConfig modifies this configuration using the given mutator.
	ChangeConfig(change agent.ConfigMutator) error
	// CurrentConfig returns the agent config for this agent.
	CurrentConfig() agent.Config
	// SetAPIHostPorts satisfies worker/apiaddressupdater/APIAddressSetter.
	SetAPIHostPorts(servers [][]network.HostPort) error
	// SetStateServingInfo satisfies worker/certupdater/SetStateServingInfo.
	SetStateServingInfo(info params.StateServingInfo) error
	// DataDir returns the directory where this agent should store its data.
	DataDir() string
}

// NewAgentConf returns a new value that satisfies AgentConf
func NewAgentConf(dataDir string) AgentConf {
	return &agentConf{dataDir: dataDir}
}

// agentConf handles command-line flags shared by all agents.
type agentConf struct {
	dataDir string
	mu      sync.Mutex
	_config agent.ConfigSetterWriter
}

// AddFlags injects common agent flags into f.
func (c *agentConf) AddFlags(f *gnuflag.FlagSet) {
	// TODO(dimitern) 2014-02-19 bug 1282025
	// We need to pass a config location here instead and
	// use it to locate the conf and the infer the data-dir
	// from there instead of passing it like that.
	f.StringVar(&c.dataDir, "data-dir", util.DataDir, "directory for juju data")
}

// CheckArgs reports whether the given args are valid for this agent.
func (c *agentConf) CheckArgs(args []string) error {
	if c.dataDir == "" {
		return util.RequiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

// DataDir returns the directory where this agent should store its data.
func (c *agentConf) DataDir() string {
	return c.dataDir
}

// ReadConfig reads the agent's config from its config file.
func (c *agentConf) ReadConfig(tag string) error {
	t, err := names.ParseTag(tag)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	conf, err := agent.ReadConfig(agent.ConfigPath(c.dataDir, t))
	if err != nil {
		return err
	}
	c._config = conf
	return nil
}

// ChangeConfig modifies this configuration using the given mutator.
func (ch *agentConf) ChangeConfig(change agent.ConfigMutator) error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if err := change(ch._config); err != nil {
		return errors.Trace(err)
	}
	if err := ch._config.Write(); err != nil {
		return errors.Annotate(err, "cannot write agent configuration")
	}
	return nil
}

// CurrentConfig returns the agent config for this agent.
func (ch *agentConf) CurrentConfig() agent.Config {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch._config.Clone()
}

// SetAPIHostPorts satisfies worker/apiaddressupdater/APIAddressSetter.
func (a *agentConf) SetAPIHostPorts(servers [][]network.HostPort) error {
	return a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetAPIHostPorts(servers)
		return nil
	})
}

// SetStateServingInfo satisfies worker/certupdater/SetStateServingInfo.
func (a *agentConf) SetStateServingInfo(info params.StateServingInfo) error {
	return a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetStateServingInfo(info)
		return nil
	})
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentVersion sets the tools version that the agent is
	// currently running.
	SetAgentVersion(v version.Binary) error
	Tag() string
	Life() state.Life
}

// isleep waits for the given duration or until it receives a value on
// stop.  It returns whether the full duration was slept without being
// stopped.
func isleep(d time.Duration, stop <-chan struct{}) bool {
	select {
	case <-stop:
		return false
	case <-time.After(d):
	}
	return true
}

type configChanger func(c *agent.Config)
