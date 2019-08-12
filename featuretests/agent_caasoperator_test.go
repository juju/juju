// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/worker/logsender"
)

const (
	initialApplicationPassword = "application-password-1234567890"
)

type CAASOperatorSuite struct {
	agenttest.AgentSuite
}

func newExecClient(modelName string) (exec.Executor, error) {
	return nil, nil
}

func (s *CAASOperatorSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
}

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	// Set up a CAAS model to replace the IAAS one.
	st := s.Factory.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	// Close the state pool before the state object itself.
	s.StatePool.Close()
	s.StatePool = nil
	err := s.State.Close()
	c.Assert(err, jc.ErrorIsNil)
	s.State = st
}

// primeAgent creates an application, and sets up the application agent's directory.
// It returns new application and the agent's configuration.
func (s *CAASOperatorSuite) primeAgent(c *gc.C) (*state.Application, agent.Config, *tools.Tools) {
	app := s.AddTestingApplication(c, "gitlab", s.AddTestingCharmForSeries(c, "gitlab", "kubernetes"))
	err := app.SetPassword(initialApplicationPassword)
	c.Assert(err, jc.ErrorIsNil)
	conf, tools := s.PrimeAgent(c, app.Tag(), initialApplicationPassword)
	return app, conf, tools
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
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForApplicationActive(c, config.DataDir(), app.Tag().String())
}

func (s *CAASOperatorSuite) TestOpenStateFails(c *gc.C) {
	app, config, _ := s.primeAgent(c)
	a := s.newAgent(c, app)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
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
	}
)

func (s *CAASOperatorSuite) newCaasOperatorAgent(c *gc.C, ctx *cmd.Context, bufferedLogger *logsender.BufferedLogWriter) (*jujudagent.CaasOperatorAgent, error) {
	a, err := jujudagent.NewCaasOperatorAgent(ctx, s.newBufferedLogWriter(), newExecClient)
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

	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysCAASWorkers, notMigratingCAASWorkers...))
	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
}
