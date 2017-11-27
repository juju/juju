// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/caasoperator"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/logsender"
)

const (
	initialApplicationPassword = "application-password-1234567890"
)

type CAASOperatorSuite struct {
	agenttest.AgentSuite
}

var _ = gc.Suite(&CAASOperatorSuite{})

// primeAgent creates an application, and sets up the application agent's directory.
// It returns new application and the agent's configuration.
func (s *CAASOperatorSuite) primeAgent(c *gc.C) (*state.Application, agent.Config, *tools.Tools) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	err := app.SetPassword(initialApplicationPassword)
	c.Assert(err, jc.ErrorIsNil)
	conf, tools := s.PrimeAgent(c, app.Tag(), initialApplicationPassword)
	return app, conf, tools
}

func (s *CAASOperatorSuite) newAgent(c *gc.C, app *state.Application) *jujudagent.CaasOperatorAgent {
	a, err := jujudagent.NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--application-name", app.Name(), "--log-to-stderr=true")
	err = a.ReadConfig(app.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	return a
}

func (s *CAASOperatorSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func waitForApplicationActive(c *gc.C, agentDir string) {
	timeout := time.After(coretesting.LongWait)

	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(coretesting.ShortWait):
			link := filepath.Join(agentDir, commands.CommandNames()[0])
			if _, err := os.Lstat(link); err == nil {
				target, err := os.Readlink(link)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(target, gc.Equals, filepath.Join(filepath.Dir(os.Args[0]), "jujud"))
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
	waitForApplicationActive(c, filepath.Join(config.DataDir(), "tools"))
}

func (s *CAASOperatorSuite) TestOpenStateFails(c *gc.C) {
	app, config, _ := s.primeAgent(c)
	a := s.newAgent(c, app)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForApplicationActive(c, filepath.Join(config.DataDir(), "tools"))

	s.AssertCannotOpenState(c, config.Tag(), config.DataDir())
}

type CAASOperatorManifoldsFunc func(config caasoperator.ManifoldsConfig) dependency.Manifolds

func TrackCAASOperator(c *gc.C, tracker *agenttest.EngineTracker, inner CAASOperatorManifoldsFunc) CAASOperatorManifoldsFunc {
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
		"api-caller",
		"operator",
	}
	notMigratingCAASWorkers = []string{
	// TODO(caas)
	}
)

func (s *CAASOperatorSuite) TestWorkers(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1610993")
	tracker := agenttest.NewEngineTracker()
	instrumented := TrackCAASOperator(c, tracker, jujudagent.CaasOperatorManifolds)
	s.PatchValue(&jujudagent.CaasOperatorManifolds, instrumented)

	app, _, _ := s.primeAgent(c)
	ctx := cmdtesting.Context(c)
	a, err := jujudagent.NewCaasOperatorAgent(ctx, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--application-name", app.Name())

	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysCAASWorkers, notMigratingCAASWorkers...))
	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
}
