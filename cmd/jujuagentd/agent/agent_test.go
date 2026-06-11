// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testhelpers"
)

type acCreator func() (cmd.Command, agentconf.AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *tc.C, dataDir string, create acCreator, args []string) cmd.Command {
	_, conf := create()
	c.Assert(conf.DataDir(), tc.Equals, dataDir)
	badArgs := append(args, "--data-dir", "")
	com, _ := create()
	err := cmdtesting.InitCommand(com, badArgs)
	c.Assert(err, tc.ErrorMatches, "--data-dir option must be set")
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

type agentLoggingSuite struct {
	testhelpers.IsolationSuite
}

func TestAgentLoggingSuite(t *testing.T) {
	tc.Run(t, &agentLoggingSuite{})
}

func (*agentLoggingSuite) TestNoLoggingConfig(c *tc.C) {
	f := &fakeLoggingConfig{}
	context := internallogger.LoggerContext(corelogger.WARNING)
	initial := context.Config().String()

	agentconf.SetupAgentLogging(context, f)

	c.Assert(context.Config().String(), tc.Equals, initial)
}

func (*agentLoggingSuite) TestLoggingOverride(c *tc.C) {
	f := &fakeLoggingConfig{
		loggingOverride: "test=INFO",
	}
	context := internallogger.LoggerContext(corelogger.WARNING)

	agentconf.SetupAgentLogging(context, f)

	c.Assert(context.Config().String(), tc.Equals, "<root>=WARNING;test=INFO")
}

func (*agentLoggingSuite) TestLoggingConfig(c *tc.C) {
	f := &fakeLoggingConfig{
		loggingConfig: "test=INFO",
	}
	context := internallogger.LoggerContext(corelogger.WARNING)

	agentconf.SetupAgentLogging(context, f)

	c.Assert(context.Config().String(), tc.Equals, "<root>=WARNING;test=INFO")
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

type machineControllerStartupValueProviderSuite struct {
	testhelpers.IsolationSuite
}

func TestMachineControllerStartupValueProviderSuite(t *testing.T) {
	tc.Run(t, &machineControllerStartupValueProviderSuite{})
}

func (s *machineControllerStartupValueProviderSuite) TestLocalValuesReadCurrentAgentConfig(c *tc.C) {
	provider := machineControllerStartupValueProvider{
		agent: &MachineAgent{AgentConfigWriter: &fakeMachineAgentConfigWriter{
			config: &fakeMachineConfig{dataDir: "/data/one", logDir: "/log/one"},
		}},
	}

	values, err := provider.LocalValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.DataDir, tc.Equals, "/data/one")
	c.Check(values.LogDir, tc.Equals, "/log/one")

	provider.agent.AgentConfigWriter = &fakeMachineAgentConfigWriter{
		config: &fakeMachineConfig{dataDir: "/data/two", logDir: "/log/two"},
	}
	values, err = provider.LocalValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.DataDir, tc.Equals, "/data/two")
	c.Check(values.LogDir, tc.Equals, "/log/two")
}

func (s *machineControllerStartupValueProviderSuite) TestCertMaterialReadsCurrentAgentConfig(c *tc.C) {
	provider := machineControllerStartupValueProvider{
		agent: &MachineAgent{AgentConfigWriter: &fakeMachineAgentConfigWriter{
			config: &fakeMachineConfig{
				caCert: "ca-one",
				controllerAgentInfo: controller.ControllerAgentInfo{
					CAPrivateKey: "ca-key-one",
					Cert:         "cert-one",
					PrivateKey:   "key-one",
				},
			},
		}},
	}

	material, err := provider.CertMaterial()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(material.CACert, tc.Equals, "ca-one")
	c.Check(material.CAPrivateKey, tc.Equals, "ca-key-one")
	c.Check(material.ControllerCert, tc.Equals, "cert-one")
	c.Check(material.ControllerPrivateKey, tc.Equals, "key-one")

	provider.agent.AgentConfigWriter = &fakeMachineAgentConfigWriter{
		config: &fakeMachineConfig{
			caCert: "ca-two",
			controllerAgentInfo: controller.ControllerAgentInfo{
				CAPrivateKey: "ca-key-two",
				Cert:         "cert-two",
				PrivateKey:   "key-two",
			},
		},
	}
	material, err = provider.CertMaterial()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(material.CACert, tc.Equals, "ca-two")
	c.Check(material.CAPrivateKey, tc.Equals, "ca-key-two")
	c.Check(material.ControllerCert, tc.Equals, "cert-two")
	c.Check(material.ControllerPrivateKey, tc.Equals, "key-two")
}

func (s *machineControllerStartupValueProviderSuite) TestControllerStartupValuesReadCurrentAgentConfig(c *tc.C) {
	provider := machineControllerStartupValueProvider{
		agent: &MachineAgent{AgentConfigWriter: &fakeMachineAgentConfigWriter{
			config: &fakeMachineConfig{
				dataDir:               "/data/one",
				caCert:                "ca-one",
				queryTracingEnabled:   true,
				queryTracingThreshold: time.Second,
				dqliteBusyTimeout:     2 * time.Second,
				dqlitePort:            17666,
				tag:                   names.NewControllerAgentTag("0"),
				controllerAgentInfo: controller.ControllerAgentInfo{
					Cert:       "cert-one",
					PrivateKey: "key-one",
				},
			},
		}},
	}

	values, err := provider.ControllerStartupValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.ControllerID, tc.Equals, "0")
	c.Check(values.DataDir, tc.Equals, "/data/one")
	c.Check(values.CACert, tc.Equals, "ca-one")
	c.Check(values.QueryTracingEnabled, tc.Equals, true)
	c.Check(values.QueryTracingThreshold, tc.Equals, time.Second)
	c.Check(values.DqliteBusyTimeout, tc.Equals, 2*time.Second)
	c.Check(values.DqlitePort, tc.Equals, 17666)
	c.Check(values.ControllerCert, tc.Equals, "cert-one")
	c.Check(values.ControllerPrivateKey, tc.Equals, "key-one")

	provider.agent.AgentConfigWriter = &fakeMachineAgentConfigWriter{
		config: &fakeMachineConfig{
			dataDir:               "/data/two",
			caCert:                "ca-two",
			queryTracingThreshold: 3 * time.Second,
			dqliteBusyTimeout:     4 * time.Second,
			tag:                   names.NewControllerAgentTag("7"),
			controllerAgentInfo: controller.ControllerAgentInfo{
				Cert:       "cert-two",
				PrivateKey: "key-two",
			},
		},
	}
	values, err = provider.ControllerStartupValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.ControllerID, tc.Equals, "7")
	c.Check(values.DataDir, tc.Equals, "/data/two")
	c.Check(values.CACert, tc.Equals, "ca-two")
	c.Check(values.QueryTracingEnabled, tc.Equals, false)
	c.Check(values.QueryTracingThreshold, tc.Equals, 3*time.Second)
	c.Check(values.DqliteBusyTimeout, tc.Equals, 4*time.Second)
	c.Check(values.DqlitePort, tc.Equals, 0)
	c.Check(values.ControllerCert, tc.Equals, "cert-two")
	c.Check(values.ControllerPrivateKey, tc.Equals, "key-two")
}

type fakeMachineAgentConfigWriter struct {
	agentconf.AgentConf
	config agent.Config
}

func (f *fakeMachineAgentConfigWriter) CurrentConfig() agent.Config {
	return f.config
}

type fakeMachineConfig struct {
	agent.Config
	dataDir               string
	logDir                string
	caCert                string
	queryTracingEnabled   bool
	queryTracingThreshold time.Duration
	dqliteBusyTimeout     time.Duration
	dqlitePort            int
	tag                   names.Tag
	controllerAgentInfo   controller.ControllerAgentInfo
}

func (f *fakeMachineConfig) DataDir() string {
	return f.dataDir
}

func (f *fakeMachineConfig) LogDir() string {
	return f.logDir
}

func (f *fakeMachineConfig) CACert() string {
	return f.caCert
}

func (f *fakeMachineConfig) QueryTracingEnabled() bool {
	return f.queryTracingEnabled
}

func (f *fakeMachineConfig) QueryTracingThreshold() time.Duration {
	return f.queryTracingThreshold
}

func (f *fakeMachineConfig) DqliteBusyTimeout() time.Duration {
	return f.dqliteBusyTimeout
}

func (f *fakeMachineConfig) DqlitePort() (int, bool) {
	return f.dqlitePort, f.dqlitePort > 0
}

func (f *fakeMachineConfig) ControllerAgentInfo() (controller.ControllerAgentInfo, bool) {
	return f.controllerAgentInfo, true
}

func (f *fakeMachineConfig) Value(key string) string {
	if key == agent.LogSinkRateLimitBurst {
		return "0"
	}
	if key == agent.LogSinkRateLimitRefill {
		return "0"
	}
	return ""
}

func (f *fakeMachineConfig) Tag() names.Tag {
	if f.tag != nil {
		return f.tag
	}
	return names.NewMachineTag("0")
}
