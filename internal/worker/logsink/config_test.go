// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
)

type ConfigTestSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConfigTestSuite{})

func (s *ConfigTestSuite) TestDefaultLogSinkConfig(c *gc.C) {
	config := DefaultLogSinkConfig()
	c.Assert(config.Validate(), jc.ErrorIsNil)
}

func (s *ConfigTestSuite) TestDefaultLogSinkConfigValidateBufferError(c *gc.C) {
	config := DefaultLogSinkConfig()
	config.LoggerBufferSize = 0
	c.Assert(config.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *ConfigTestSuite) TestDefaultLogSinkConfigValidateIntervalError(c *gc.C) {
	config := DefaultLogSinkConfig()
	config.LoggerFlushInterval = time.Minute
	c.Assert(config.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *ConfigTestSuite) TestEmptyAgentConfig(c *gc.C) {
	agentConfig := FakeConfig{
		values: map[string]string{},
	}
	logSinkConfig, err := getLogSinkConfig(agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(logSinkConfig, gc.DeepEquals, DefaultLogSinkConfig())
}

func (s *ConfigTestSuite) TestAgentConfig(c *gc.C) {
	agentConfig := FakeConfig{
		values: map[string]string{
			agent.LogSinkLoggerBufferSize:    "42",
			agent.LogSinkLoggerFlushInterval: "8s",
		},
	}
	logSinkConfig, err := getLogSinkConfig(agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(logSinkConfig, gc.DeepEquals, LogSinkConfig{
		LoggerBufferSize:    42,
		LoggerFlushInterval: time.Second * 8,
	})
}

func (s *ConfigTestSuite) TestAgentConfigBufferError(c *gc.C) {
	agentConfig := FakeConfig{
		values: map[string]string{
			agent.LogSinkLoggerBufferSize: "!!!",
		},
	}
	_, err := getLogSinkConfig(agentConfig)
	c.Assert(err, gc.ErrorMatches, `parsing LOGSINK_LOGGER_BUFFER_SIZE.*`)
}

func (s *ConfigTestSuite) TestAgentConfigIntervalError(c *gc.C) {
	agentConfig := FakeConfig{
		values: map[string]string{
			agent.LogSinkLoggerBufferSize:    "42",
			agent.LogSinkLoggerFlushInterval: "!!!",
		},
	}
	_, err := getLogSinkConfig(agentConfig)
	c.Assert(err, gc.ErrorMatches, `parsing LOGSINK_LOGGER_FLUSH_INTERVAL.*`)
}

type FakeConfig struct {
	agent.ConfigSetter
	values map[string]string
}

func (f FakeConfig) Value(key string) string {
	if f.values == nil {
		return ""
	}
	return f.values[key]
}
