// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testhelpers"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/gate"
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
	runtimePath := filepath.Join(runtimeDir, controllerruntimeconfig.Filename)
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
		app:                   &ControllerApplication{},
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
	runtimePath := filepath.Join(runtimeDir, controllerruntimeconfig.Filename)
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
		app:                   &ControllerApplication{},
		controllerRuntimePath: runtimePath,
	}

	override, err := provider.LoggingOverride()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(override, tc.Equals, "test=INFO")
}

func (s *controllerStartupValueProviderSuite) TestLoggingOverrideReturnsRuntimeConfigError(c *tc.C) {
	provider := controllerStartupValueProvider{
		app:                   &ControllerApplication{},
		controllerRuntimePath: filepath.Join(c.MkDir(), "missing-runtime.conf"),
	}

	_, err := provider.LoggingOverride()
	c.Assert(err, tc.ErrorMatches, `reading controller runtime config ".*missing-runtime.conf": open .*missing-runtime.conf: no such file or directory`)
}

func (s *controllerStartupValueProviderSuite) TestSystemIdentityValuesUseCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, controllerruntimeconfig.Filename)
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
		app:                   &ControllerApplication{},
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

func (s *controllerStartupValueProviderSuite) TestStandaloneControllerLocks(c *tc.C) {
	app := &ControllerApplication{}
	app.initStandaloneControllerLocks()

	c.Check(app.bootstrapLock.IsUnlocked(), tc.IsFalse)
	c.Check(app.controllerUpgradeLock.IsUnlocked(), tc.IsTrue)
	c.Check(app.upgradeDBLock.IsUnlocked(), tc.IsTrue)
	c.Check(app.upgradeStepsLock.IsUnlocked(), tc.IsTrue)
	c.Check(app.upgradeCheckLock.IsUnlocked(), tc.IsTrue)

	_, ok := app.upgradeDBLock.(gate.AlreadyUnlocked)
	c.Check(ok, tc.IsTrue)
}

func (s *controllerStartupValueProviderSuite) TestCACertReadsCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, controllerruntimeconfig.Filename)
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
		app:                   &ControllerApplication{},
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
		app:                   &ControllerApplication{},
		controllerRuntimePath: filepath.Join(c.MkDir(), "missing-runtime.conf"),
	}

	_, err := provider.CACert()
	c.Assert(err, tc.ErrorMatches, `reading controller runtime config ".*missing-runtime.conf": open .*missing-runtime.conf: no such file or directory`)
}

func (s *controllerStartupValueProviderSuite) TestCurrentSnapshotReadsCurrentRuntimeConfig(c *tc.C) {
	runtimeDir := c.MkDir()
	runtimePath := filepath.Join(runtimeDir, controllerruntimeconfig.Filename)
	insecure := true
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:           "0",
		ControllerUUID:         "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:    "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:                filepath.Join(runtimeDir, "data-one"),
		LogDir:                 filepath.Join(runtimeDir, "log-one"),
		APIPort:                17070,
		AgentPassword:          "agent-password",
		LokiEndpoint:           "https://loki.one.example/loki/api/v1/push",
		LokiCACert:             "ca-one",
		LokiInsecureSkipVerify: &insecure,
		LokiOrgID:              "org-one",
		CACert:                 "ca-cert",
		CAPrivateKey:           "ca-key",
		ControllerCert:         "server-cert",
		ControllerPrivateKey:   "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	provider := controllerStartupValueProvider{
		app:                   &ControllerApplication{},
		controllerRuntimePath: runtimePath,
	}

	snapshot, err := provider.CurrentLokiConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(snapshot.Endpoint, tc.Equals, "https://loki.one.example/loki/api/v1/push")
	c.Check(snapshot.CACertificate, tc.Equals, "ca-one")
	c.Assert(snapshot.InsecureSkipVerify, tc.NotNil)
	c.Check(*snapshot.InsecureSkipVerify, tc.IsTrue)
	c.Check(snapshot.ControllerUUID, tc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(snapshot.ModelUUID, tc.Equals, "feedface-dead-beef-cafe-c0ffee000000")
	c.Check(snapshot.AgentID, tc.Equals, names.NewControllerAgentTag("0").String())
	c.Check(snapshot.OrgID, tc.Equals, "org-one")

	err = controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:           "0",
		ControllerUUID:         "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:    "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:                filepath.Join(runtimeDir, "data-two"),
		LogDir:                 filepath.Join(runtimeDir, "log-two"),
		APIPort:                17070,
		AgentPassword:          "agent-password",
		LokiEndpoint:           "",
		LokiCACert:             "",
		LokiInsecureSkipVerify: nil,
		LokiOrgID:              "",
		CACert:                 "ca-cert",
		CAPrivateKey:           "ca-key",
		ControllerCert:         "server-cert",
		ControllerPrivateKey:   "server-key",
	})
	c.Assert(err, tc.ErrorIsNil)

	snapshot, err = provider.CurrentLokiConfig()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(snapshot.Endpoint, tc.Equals, "")
	c.Check(snapshot.CACertificate, tc.Equals, "")
	c.Check(snapshot.InsecureSkipVerify, tc.IsNil)
	c.Check(snapshot.OrgID, tc.Equals, "")
}

func (s *controllerStartupValueProviderSuite) TestCurrentSnapshotReturnsRuntimeConfigError(c *tc.C) {
	provider := controllerStartupValueProvider{
		app:                   &ControllerApplication{},
		controllerRuntimePath: filepath.Join(c.MkDir(), "missing-runtime.conf"),
	}

	_, err := provider.CurrentLokiConfig()
	c.Assert(err, tc.ErrorMatches, `reading controller runtime config ".*missing-runtime.conf": open .*missing-runtime.conf: no such file or directory`)
}

type controllerLifecycleSuite struct {
	testhelpers.IsolationSuite
}

func TestControllerLifecycleSuite(t *testing.T) {
	tc.Run(t, &controllerLifecycleSuite{})
}

// TestSigtermLogicCleanExit tests the extracted signal-handling logic in
// isolation: a channel-driven goroutine kills the runner on signal receipt
// and the result is translated to a clean exit. This is a fast unit check
// of the runner/AgentDone interaction without going through signal.Notify.
func (s *controllerLifecycleSuite) TestSigtermLogicCleanExit(c *tc.C) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "test-controller",
		IsFatal:       func(error) bool { return false },
		MoreImportant: func(err0, err1 error) bool { return false },
		RestartDelay:  internalworker.RestartDelay,
		Logger:        internalworker.WrapLogger(logger),
	})
	c.Assert(err, tc.ErrorIsNil)

	app := &ControllerApplication{
		runner: runner,
		dead:   make(chan struct{}),
	}

	blk := internalworker.NewSimpleWorker(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})
	err = runner.StartWorker(c.Context(), "blocker",
		func(context.Context) (worker.Worker, error) { return blk, nil })
	c.Assert(err, tc.ErrorIsNil)

	sigCh := make(chan os.Signal, 1)
	var stoppedBySigterm atomic.Bool
	go func() {
		select {
		case <-sigCh:
			stoppedBySigterm.Store(true)
			runner.Kill()
		case <-app.dead:
		}
	}()

	sigCh <- syscall.SIGTERM

	runnerErr := runner.Wait()
	if stoppedBySigterm.Load() {
		runnerErr = cmdutil.AgentDone(logger, internalworker.ErrTerminateAgent)
	} else {
		runnerErr = cmdutil.AgentDone(logger, runnerErr)
	}
	c.Check(runnerErr, tc.ErrorIsNil)
}

// TestSigtermWiringCleanExit calls app.Run() and delivers a real SIGTERM to
// the process to exercise the signal.Notify registration inside Run. It
// asserts that Run returns nil, confirming that the OS-delivered signal
// reaches the handler, kills the runner, and is translated to a clean exit.
func (s *controllerLifecycleSuite) TestSigtermWiringCleanExit(c *tc.C) {
	dir := c.MkDir()
	logDir := filepath.Join(dir, "log")
	c.Assert(os.MkdirAll(logDir, 0o755), tc.ErrorIsNil)

	runtimePath := filepath.Join(dir, controllerruntimeconfig.Filename)
	err := controllerruntimeconfig.WriteControllerRuntimeConfig(runtimePath, controllerruntimeconfig.ControllerRuntimeConfig{
		ControllerID:         "0",
		ControllerUUID:       "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ControllerModelUUID:  "feedface-dead-beef-cafe-c0ffee000000",
		DataDir:              dir,
		LogDir:               logDir,
		APIPort:              17070,
		AgentPassword:        "agent-password",
		CACert:               "ca-cert-pem",
		CAPrivateKey:         "ca-private-key-pem",
		ControllerCert:       "controller-cert-pem",
		ControllerPrivateKey: "controller-private-key-pem",
	})
	c.Assert(err, tc.ErrorIsNil)

	agentTag := names.NewControllerAgentTag("0")
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "test-controller",
		IsFatal:       func(error) bool { return false },
		MoreImportant: func(err0, err1 error) bool { return false },
		RestartDelay:  internalworker.RestartDelay,
		Logger:        internalworker.WrapLogger(logger),
	})
	c.Assert(err, tc.ErrorIsNil)

	app := &ControllerApplication{
		agentTag:              agentTag,
		runner:                runner,
		controllerRuntimePath: runtimePath,
		workersStarted:        make(chan struct{}),
		dead:                  make(chan struct{}),
		// Inject a minimal blocking worker so the runner stays alive
		// until killed, without starting the full dependency engine.
		testEngineCreator: func(ctx context.Context) (worker.Worker, error) {
			return internalworker.NewSimpleWorker(func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			}), nil
		},
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(&cmd.Context{Context: c.Context()})
	}()

	// Wait until signal.Notify is registered (workersStarted is closed
	// just before the signal handler goroutine is launched).
	select {
	case <-app.WorkersStarted():
	case <-c.Context().Done():
		c.Fatal("timed out waiting for workers to start")
	}

	// Deliver a real SIGTERM to this process to exercise the wiring.
	c.Assert(syscall.Kill(os.Getpid(), syscall.SIGTERM), tc.ErrorIsNil)

	select {
	case result := <-runErr:
		c.Check(result, tc.ErrorIsNil)
	case <-c.Context().Done():
		c.Fatal("timed out waiting for Run to return")
	}
}

func (s *controllerLifecycleSuite) TestRunReturnsErrorWhenRuntimeConfigIsMissing(c *tc.C) {
	agentTag := names.NewControllerAgentTag("0")
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:          "test-controller",
		IsFatal:       func(error) bool { return false },
		MoreImportant: func(err0, err1 error) bool { return false },
		Logger:        internalworker.WrapLogger(logger),
	})
	c.Assert(err, tc.ErrorIsNil)

	app := &ControllerApplication{
		agentTag:              agentTag,
		runner:                runner,
		controllerRuntimePath: filepath.Join(c.MkDir(), "nonexistent-runtime.conf"),
		workersStarted:        make(chan struct{}),
		dead:                  make(chan struct{}),
	}

	cmdCtx := &cmd.Context{
		Context: context.Background(),
	}
	result := app.Run(cmdCtx)

	c.Check(result, tc.ErrorMatches, `reading controller runtime config ".*nonexistent-runtime.conf": open .*: no such file or directory`)
}
