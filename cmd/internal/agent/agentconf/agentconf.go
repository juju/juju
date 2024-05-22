// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconf

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/config"
	agenterrors "github.com/juju/juju/agent/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/state/mgo"
)

// AgentConf is a terribly confused interface.
//
// Parts of it are a mixin for cmd.Command implementations; others are a mixin
// for agent.Agent implementations; others bridge the two. We should be aiming
// to separate the cmd responsibilities from the agent responsibilities.
type AgentConf interface {
	config.AgentConf

	// AddFlags injects common agent flags into f.
	AddFlags(f *gnuflag.FlagSet)

	// CheckArgs reports whether the given args are valid for this agent.
	CheckArgs(args []string) error
}

// NewAgentConf returns a new value that satisfies AgentConf
func NewAgentConf(dataDir string) AgentConf {
	return &agentConf{
		AgentConfig: config.NewAgentConfig(dataDir),
	}
}

// agentConf handles command-line flags shared by all agents.
type agentConf struct {
	*config.AgentConfig
}

// AddFlags injects common agent flags into f.
func (c *agentConf) AddFlags(f *gnuflag.FlagSet) {
	// TODO(dimitern) 2014-02-19 bug 1282025
	// We need to pass a config location here instead and
	// use it to locate the conf and the infer the data-dir
	// from there instead of passing it like that.
	f.StringVar(&c.RawDataDir, "data-dir", config.DataDir, "directory for juju data")
}

// CheckArgs reports whether the given args are valid for this agent.
func (c *agentConf) CheckArgs(args []string) error {
	if c.DataDir() == "" {
		return agenterrors.RequiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

func SetupAgentLogging(context corelogger.LoggerContext, config agent.Config) {
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
