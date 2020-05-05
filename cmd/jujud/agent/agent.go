// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
agent contains jujud's machine agent.
*/
package agent

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/version"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/util"
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

func setupAgentLogging(config agent.Config) {

	if loggingOverride := config.Value(agent.LoggingOverride); loggingOverride != "" {
		logger.Infof("logging override set for this agent: %q", loggingOverride)
		loggo.DefaultContext().ResetLoggerLevels()
		err := loggo.ConfigureLoggers(loggingOverride)
		if err != nil {
			logger.Errorf("setting logging override %v", err)
		}
	} else if loggingConfig := config.LoggingConfig(); loggingConfig != "" {
		logger.Infof("setting logging config to %q", loggingConfig)
		// There should only be valid logging configuration strings saved
		// in the logging config section in the agent.conf file.
		loggo.DefaultContext().ResetLoggerLevels()
		err := loggo.ConfigureLoggers(loggingConfig)
		if err != nil {
			logger.Errorf("problem setting logging config %v", err)
		}
	}

	if flags := featureflag.String(); flags != "" {
		logger.Warningf("developer feature flags enabled: %s", flags)
	}
}

// GetJujuVersion gets the version of the agent from agent's config file
func GetJujuVersion(machineAgent string, dataDir string) (version.Number, error) {
	agentConf := NewAgentConf(dataDir)
	if err := agentConf.ReadConfig(machineAgent); err != nil {
		err = errors.Annotate(err, "failed to read agent config file.")
		return version.Number{}, err
	}
	config := agentConf.CurrentConfig()
	if config == nil {
		return version.Number{}, errors.Errorf("%s agent conf is not found", machineAgent)
	}
	return config.UpgradedToVersion(), nil
}

func dependencyEngineConfig() dependency.EngineConfig {
	return dependency.EngineConfig{
		IsFatal:          util.IsFatal,
		WorstError:       util.MoreImportantError,
		ErrorDelay:       3 * time.Second,
		BounceDelay:      10 * time.Millisecond,
		BackoffFactor:    1.2,
		BackoffResetTime: 1 * time.Minute,
		MaxDelay:         2 * time.Minute,
		Clock:            clock.WallClock,
		Logger:           loggo.GetLogger("juju.worker.dependency"),
	}
}

// readAgentConfig is a helper to read either machine or controller agent config,
// whichever is there. Machine config gets precedence.
func readAgentConfig(c AgentConfigWriter, agentId string) error {
	var tag names.Tag = names.NewMachineTag(agentId)
	err := c.ReadConfig(tag.String())
	if err != nil {
		tag = names.NewControllerAgentTag(agentId)
		err = c.ReadConfig(tag.String())
	}
	return errors.Trace(err)
}
