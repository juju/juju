// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	caasoperatorworker "github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/uniter"
)

const (
	initialApplicationPassword = "application-password-1234567890"
)

type CAASOperatorSuite struct {
	agenttest.AgentSuite
}

func newExecClient(modelName string) (exec.Executor, error) {
	return &mockExecutor{}, nil
}

func (s *CAASOperatorSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
}

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	// Set up a CAAS model to replace the IAAS one.
	st := s.Factory.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	s.State = st

	os.Setenv("JUJU_OPERATOR_SERVICE_IP", "127.0.0.1")
	os.Setenv("JUJU_OPERATOR_POD_IP", "127.0.0.1")
}

func (s *CAASOperatorSuite) TearDownTest(c *gc.C) {
	os.Setenv("JUJU_OPERATOR_SERVICE_IP", "")
	os.Setenv("JUJU_OPERATOR_POD_IP", "")

	s.AgentSuite.TearDownTest(c)
}

// primeAgent creates an application, and sets up the application agent's directory.
// It returns new application and the agent's configuration.
func (s *CAASOperatorSuite) primeAgent(c *gc.C) (*state.Application, agent.Config, *tools.Tools) {
	app := s.AddTestingApplication(c, "gitlab", s.AddTestingCharmForSeries(c, "gitlab", "kubernetes"))
	err := app.SetPassword(initialApplicationPassword)
	c.Assert(err, jc.ErrorIsNil)
	conf, tools := s.PrimeAgent(c, app.Tag(), initialApplicationPassword)
	s.primeOperator(c, app)
	return app, conf, tools
}

func (s *CAASOperatorSuite) primeOperator(c *gc.C, app *state.Application) {
	baseDir := agent.Dir(s.DataDir(), app.Tag())
	file := filepath.Join(baseDir, caas.OperatorInfoFile)
	info := caas.OperatorInfo{
		CACert:     coretesting.CACert,
		Cert:       coretesting.ServerCert,
		PrivateKey: coretesting.ServerKey,
	}
	data, err := info.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(file, data, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CAASOperatorSuite) newAgent(c *gc.C, app *state.Application) *jujudagent.CaasOperatorAgent {
	a, err := s.newCaasOperatorAgent(c, nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--application-name", app.Name())
	err = a.ReadConfig(app.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	return a
}

func (s *CAASOperatorSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func waitForApplicationActive(c *gc.C, dataDir, appTag string) {
	timeout := time.After(coretesting.LongWait)
	agentCharmDir := filepath.Join(dataDir, "agents", appTag, "charm")
	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(coretesting.ShortWait):
			if _, err := os.Stat(agentCharmDir); err == nil {
				return
			}
		}
	}
}

func (s *CAASOperatorSuite) TestRunStop(c *gc.C) {
	app, config, _ := s.primeAgent(c)
	a := s.newAgent(c, app)
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForApplicationActive(c, config.DataDir(), app.Tag().String())
}

func (s *CAASOperatorSuite) TestOpenStateFails(c *gc.C) {
	app, config, _ := s.primeAgent(c)
	a := s.newAgent(c, app)
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForApplicationActive(c, config.DataDir(), app.Tag().String())

	s.AssertCannotOpenState(c, config.Tag(), config.DataDir())
}

type CaasOperatorManifoldsFunc func(config caasoperator.ManifoldsConfig) dependency.Manifolds

func TrackCAASOperator(c *gc.C, tracker *agenttest.EngineTracker, inner CaasOperatorManifoldsFunc) CaasOperatorManifoldsFunc {
	return func(config caasoperator.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}

var (
	alwaysCAASWorkers = []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"api-config-watcher",
		"clock",
		"logging-config-updater",
		"log-sender",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
	}
	notMigratingCAASWorkers = []string{
		"charm-dir",
		"hook-retry-strategy",
		"operator",
		"proxy-config-updater",
	}
)

func sockPath(c *gc.C) sockets.Socket {
	sockPath := filepath.Join(c.MkDir(), "test.listener")
	if runtime.GOOS == "windows" {
		return sockets.Socket{Address: `\\.\pipe` + sockPath[2:], Network: "unix"}
	}
	return sockets.Socket{Address: sockPath, Network: "unix"}
}

func (s *CAASOperatorSuite) newCaasOperatorAgent(c *gc.C, ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (*jujudagent.CaasOperatorAgent, error) {
	configure := func(mc *caasoperator.ManifoldsConfig) error {
		mc.NewExecClient = newExecClient
		mc.RunListenerSocket = func(*uniter.SocketConfig) (*sockets.Socket, error) {
			socket := sockPath(c)
			return &socket, nil
		}
		mc.NewContainerStartWatcherClient = func(_ caasoperatorworker.Client) caasoperatorworker.ContainerStartWatcher {
			return &mockContainerStartWatcher{}
		}
		return nil
	}
	a, err := jujudagent.NewCaasOperatorAgent(ctx, s.newBufferedLogWriter(), configure)
	c.Assert(err, jc.ErrorIsNil)
	return a, nil
}

func (s *CAASOperatorSuite) TestWorkers(c *gc.C) {
	tracker := agenttest.NewEngineTracker()
	instrumented := TrackCAASOperator(c, tracker, jujudagent.CaasOperatorManifolds)
	s.PatchValue(&jujudagent.CaasOperatorManifolds, instrumented)

	app, _, _ := s.primeAgent(c)
	ctx := cmdtesting.Context(c)
	a, err := s.newCaasOperatorAgent(c, ctx, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--application-name", app.Name())

	go func() { c.Check(a.Run(ctx), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysCAASWorkers, notMigratingCAASWorkers...))
	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
}

type mockContainerStartWatcher struct{}

func (*mockContainerStartWatcher) WatchContainerStart(appName string, container string) (watcher.StringsWatcher, error) {
	return watchertest.NewMockStringsWatcher(make(chan []string)), nil
}

type mockExecutor struct{}

var _ exec.Executor = &mockExecutor{}

func (*mockExecutor) Status(params exec.StatusParams) (*exec.Status, error) {
	return nil, errors.NotImplementedf("exec status")
}

func (*mockExecutor) Exec(params exec.ExecParams, cancel <-chan struct{}) error {
	return errors.NotImplementedf("exec")
}

func (*mockExecutor) Copy(params exec.CopyParams, cancel <-chan struct{}) error {
	return errors.NotImplementedf("exec copy")
}
