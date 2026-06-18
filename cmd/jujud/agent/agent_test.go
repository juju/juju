// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"path/filepath"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/controllerruntimeconfig"
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

type controllerStartupValueProviderSuite struct {
	testhelpers.IsolationSuite
}

func TestControllerStartupValueProviderSuite(t *testing.T) {
	tc.Run(t, &controllerStartupValueProviderSuite{})
}

func (s *controllerStartupValueProviderSuite) TestLoggingOverrideReadsCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, "runtime.conf")
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              filepath.Join(runtimeDir, "data-one"),
		LogDir:               filepath.Join(runtimeDir, "log-one"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		LoggingConfig:        "first",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	provider := controllerStartupValueProvider{
		agent: &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{
			config: &fakeControllerConfig{loggingConfig: "first"},
		}},
		controllerRuntimePath: runtimePath,
	}

	override, err := provider.LoggingOverride()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(override, tc.Equals, "first")

	err = controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              filepath.Join(runtimeDir, "data-two"),
		LogDir:               filepath.Join(runtimeDir, "log-two"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		LoggingConfig:        "second",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	override, err = provider.LoggingOverride()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(override, tc.Equals, "second")
}

func (s *controllerStartupValueProviderSuite) TestLoggingOverrideFieldTakesPrecedence(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, "runtime.conf")
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              filepath.Join(runtimeDir, "data-one"),
		LogDir:               filepath.Join(runtimeDir, "log-one"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		LoggingConfig:        "<root>=WARNING",
		LoggingOverride:      "test=INFO",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	provider := controllerStartupValueProvider{
		agent: &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{
			config: &fakeControllerConfig{
				loggingOverride: "ignored",
			},
		}},
		controllerRuntimePath: runtimePath,
	}

	override, err := provider.LoggingOverride()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(override, tc.Equals, "test=INFO")
}

func (s *controllerStartupValueProviderSuite) TestLoggingOverrideReturnsRuntimeConfigError(c *tc.C) {
	provider := controllerStartupValueProvider{
		agent: &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{
			config: &fakeControllerConfig{},
		}},
		controllerRuntimePath: filepath.Join(c.MkDir(), "missing-runtime.conf"),
	}

	_, err := provider.LoggingOverride()
	c.Assert(err, tc.ErrorMatches, `reading controller runtime config ".*missing-runtime.conf": open .*missing-runtime.conf: no such file or directory`)
}

func (s *controllerStartupValueProviderSuite) TestSystemIdentityValuesUseCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, "runtime.conf")
	dataDirOne := filepath.Join(runtimeDir, "data-one")
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              dataDirOne,
		LogDir:               filepath.Join(runtimeDir, "log-one"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
		SystemIdentity:       "identity-one",
	})
	c.Assert(err, tc.ErrorIsNil)

	provider := controllerStartupValueProvider{
		agent:                 &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{}},
		controllerRuntimePath: runtimePath,
	}

	values, err := provider.SystemIdentityValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.SystemIdentity, tc.Equals, "identity-one")
	c.Check(values.SystemIdentityPath, tc.Equals, filepath.Join(dataDirOne, agent.SystemIdentity))

	dataDirTwo := filepath.Join(runtimeDir, "data-two")
	err = controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              dataDirTwo,
		LogDir:               filepath.Join(runtimeDir, "log-two"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
		SystemIdentity:       "identity-two",
	})
	c.Assert(err, tc.ErrorIsNil)

	values, err = provider.SystemIdentityValues()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(values.SystemIdentity, tc.Equals, "identity-two")
	c.Check(values.SystemIdentityPath, tc.Equals, filepath.Join(dataDirTwo, agent.SystemIdentity))
}

type fakeAgentConfigWriter struct {
	agentconf.AgentConf
	config agent.Config
}

func (f *fakeAgentConfigWriter) CurrentConfig() agent.Config {
	return f.config
}

type fakeControllerConfig struct {
	agent.Config
	loggingConfig   string
	loggingOverride string
	caCert          string
}

func (f *fakeControllerConfig) LoggingConfig() string {
	return f.loggingConfig
}

func (f *fakeControllerConfig) Value(key string) string {
	if key == agent.LoggingOverride {
		return f.loggingOverride
	}
	return ""
}

func (f *fakeControllerConfig) Tag() names.Tag {
	return names.NewControllerAgentTag("0")
}

func (f *fakeControllerConfig) Model() names.ModelTag {
	return names.NewModelTag("model-uuid")
}

func (f *fakeControllerConfig) CACert() string {
	return f.caCert
}

func (s *controllerStartupValueProviderSuite) TestCACertReadsCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, "runtime.conf")
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              filepath.Join(runtimeDir, "data-one"),
		LogDir:               filepath.Join(runtimeDir, "log-one"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-one",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	provider := controllerStartupValueProvider{
		agent:                 &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{}},
		controllerRuntimePath: runtimePath,
	}

	caCert, err := provider.CACert()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caCert, tc.Equals, "ca-one")

	err = controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              filepath.Join(runtimeDir, "data-two"),
		LogDir:               filepath.Join(runtimeDir, "log-two"),
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-two",
		CAPrivateKey:         "ca-key",
		ControllerCert:       "server-cert",
		ControllerPrivateKey: "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	caCert, err = provider.CACert()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caCert, tc.Equals, "ca-two")
}

func (s *controllerStartupValueProviderSuite) TestCACertReturnsRuntimeConfigError(c *tc.C) {
	provider := controllerStartupValueProvider{
		agent:                 &ControllerAgent{AgentConfigWriter: &fakeAgentConfigWriter{}},
		controllerRuntimePath: filepath.Join(c.MkDir(), "missing-runtime.conf"),
	}

	_, err := provider.CACert()
	c.Assert(err, tc.ErrorMatches, `reading controller runtime config ".*missing-runtime.conf": open .*missing-runtime.conf: no such file or directory`)
}
