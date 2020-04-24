// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/core/network"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/worker/proxyupdater"
)

type acCreator func() (cmd.Command, AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *gc.C, dataDir string, create acCreator, args []string) cmd.Command {
	_, conf := create()
	c.Assert(conf.DataDir(), gc.Equals, dataDir)
	badArgs := append(args, "--data-dir", "")
	com, _ := create()
	err := cmdtesting.InitCommand(com, badArgs)
	c.Assert(err, gc.ErrorMatches, "--data-dir option must be set")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--data-dir", "jd",
	}
	return cmdtesting.InitCommand(ac, append(common, args...))
}

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	agenttest.AgentSuite
}

func (s *AgentSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	agenttest.InstallFakeEnsureMongo(s)
}

func (s *AgentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Set API host ports so FindTools/Tools API calls succeed.
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&proxyupdater.NewWorker, func(proxyupdater.Config) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

	// Tests should not try to use internet. Ensure base url is empty.
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
}

type agentLoggingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&agentLoggingSuite{})

func (*agentLoggingSuite) TestNoLoggingConfig(c *gc.C) {
	f := &fakeLoggingConfig{}
	initial := loggo.LoggerInfo()

	setupAgentLogging(f)

	c.Assert(loggo.LoggerInfo(), gc.Equals, initial)
}

func (*agentLoggingSuite) TestLoggingOverride(c *gc.C) {
	f := &fakeLoggingConfig{
		loggingOverride: "test=INFO",
	}

	setupAgentLogging(f)

	c.Assert(loggo.LoggerInfo(), gc.Equals, "<root>=WARNING;test=INFO")
}

func (*agentLoggingSuite) TestLoggingConfig(c *gc.C) {
	f := &fakeLoggingConfig{
		loggingConfig: "test=INFO",
	}

	setupAgentLogging(f)

	c.Assert(loggo.LoggerInfo(), gc.Equals, "<root>=WARNING;test=INFO")
}

type fakeLoggingConfig struct {
	agent.Config

	loggingConfig   string
	loggingOverride string
}

func (f *fakeLoggingConfig) LoggingConfig() string {
	return f.loggingConfig
}

func (f *fakeLoggingConfig) Value(key string) string {
	if key == agent.LoggingOverride {
		return f.loggingOverride
	}
	return ""
}
