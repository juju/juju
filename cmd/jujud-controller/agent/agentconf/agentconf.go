// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconf

import (
	"sync"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud-controller/agent/config"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/state/mgo"
)

// AgentConf is a terribly confused interface.
//
// Parts of it are a mixin for cmd.Command implementations; others are a mixin
// for agent.Agent implementations; others bridge the two. We should be aiming
// to separate the cmd responsibilities from the agent responsibilities.
type AgentConf interface {

	// AddFlags injects common agent flags into f.
	AddFlags(f *gnuflag.FlagSet)

	// CheckArgs reports whether the given args are valid for this agent.
	CheckArgs(args []string) error

	// DataDir returns the directory where this agent should store its data.
	DataDir() string

	// ReadConfig reads the agent's config from its config file.
	ReadConfig(tag string) error

	// CurrentConfig returns the agent config for this agent.
	CurrentConfig() agent.Config

	// ChangeConfig modifies this configuration using the given mutator.
	ChangeConfig(change agent.ConfigMutator) error
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
	f.StringVar(&c.dataDir, "data-dir", config.DataDir, "directory for juju data")
}

// CheckArgs reports whether the given args are valid for this agent.
func (c *agentConf) CheckArgs(args []string) error {
	if c.dataDir == "" {
		return agenterrors.RequiredError("data-dir")
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
		return errors.Trace(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	conf, err := agent.ReadConfig(agent.ConfigPath(c.dataDir, t))
	if err != nil {
		return errors.Trace(err)
	}
	c._config = conf
	return nil
}

// ChangeConfig modifies this configuration using the given mutator.
func (c *agentConf) ChangeConfig(change agent.ConfigMutator) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := change(c._config); err != nil {
		return errors.Trace(err)
	}
	if err := c._config.Write(); err != nil {
		return errors.Annotate(err, "cannot write agent configuration")
	}
	return nil
}

// CurrentConfig returns the agent config for this agent.
func (c *agentConf) CurrentConfig() agent.Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c._config.Clone()
}

func SetupAgentLogging(context *loggo.Context, config agent.Config) {
	logger := context.GetLogger("juju.agent.setup")
	if loggingOverride := config.Value(agent.LoggingOverride); loggingOverride != "" {
		logger.Infof("logging override set for this agent: %q", loggingOverride)
		context.ResetLoggerLevels()
		err := context.ConfigureLoggers(loggingOverride)
		if err != nil {
			logger.Errorf("setting logging override %v", err)
		}
	} else if loggingConfig := config.LoggingConfig(); loggingConfig != "" {
		logger.Infof("setting logging config to %q", loggingConfig)
		// There should only be valid logging configuration strings saved
		// in the logging config section in the agent.conf file.
		context.ResetLoggerLevels()
		err := context.ConfigureLoggers(loggingConfig)
		if err != nil {
			logger.Errorf("problem setting logging config %v", err)
		}
		mgo.ConfigureMgoLogging()
	}

	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}
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
