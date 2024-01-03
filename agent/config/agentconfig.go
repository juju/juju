// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
)

// AgentConf is an interface that provides access to the agent's configuration.
type AgentConf interface {

	// DataDir returns the directory where this agent should store its data.
	DataDir() string

	// ReadConfig reads the agent's config from its config file.
	ReadConfig(tag string) error

	// CurrentConfig returns the agent config for this agent.
	CurrentConfig() agent.Config

	// ChangeConfig modifies this configuration using the given mutator.
	ChangeConfig(change agent.ConfigMutator) error
}

// AgentConfig implements AgentConf, it is expected to be embedded in
// other types that need to implement AgentConf.
type AgentConfig struct {
	RawDataDir string
	mu         sync.Mutex
	config     agent.ConfigSetterWriter
}

// NewAgentConfig returns a new AgentConfig.
func NewAgentConfig(dataDir string) *AgentConfig {
	return &AgentConfig{
		RawDataDir: dataDir,
	}
}

// NewAgentConfig returns a new AgentConfig.
func NewAgentConfigWithConfigSetterWriter(dataDir string, config agent.ConfigSetterWriter) *AgentConfig {
	return &AgentConfig{
		RawDataDir: dataDir,
		config:     config,
	}
}

// DataDir returns the directory where this agent should store its data.
func (c *AgentConfig) DataDir() string {
	return c.RawDataDir
}

// ReadConfig reads the agent's config from its config file.
func (c *AgentConfig) ReadConfig(tag string) error {
	t, err := names.ParseTag(tag)
	if err != nil {
		return errors.Trace(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	conf, err := agent.ReadConfig(agent.ConfigPath(c.RawDataDir, t))
	if err != nil {
		return errors.Trace(err)
	}
	c.config = conf
	return nil
}

// ChangeConfig modifies this configuration using the given mutator.
func (c *AgentConfig) ChangeConfig(change agent.ConfigMutator) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := change(c.config); err != nil {
		return errors.Trace(err)
	}
	if err := c.config.Write(); err != nil {
		return errors.Annotate(err, "cannot write agent configuration")
	}
	return nil
}

// CurrentConfig returns the agent config for this agent.
func (c *AgentConfig) CurrentConfig() agent.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.config.Clone()
}

// AgentConfigWriter encapsulates disk I/O operations with the agent
// config.
type AgentConfigWriter interface {
	// ReadConfig reads the config for the given tag from disk.
	ReadConfig(tag string) error
	// ChangeConfig executes the given agent.ConfigMutator in a
	// thread-safe context.
	ChangeConfig(agent.ConfigMutator) error
	// CurrentConfig returns a copy of the in-memory agent config.
	CurrentConfig() agent.Config
}

// ReadAgentConfig is a helper to read either machine or controller agent config,
// whichever is there. Machine config gets precedence.
func ReadAgentConfig(c AgentConfigWriter, agentId string) error {
	var tag names.Tag = names.NewMachineTag(agentId)
	err := c.ReadConfig(tag.String())
	if err != nil {
		tag = names.NewControllerAgentTag(agentId)
		err = c.ReadConfig(tag.String())
	}
	return errors.Trace(err)
}
