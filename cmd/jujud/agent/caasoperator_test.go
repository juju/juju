// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/logsender"
)

type CAASOperatorSuite struct {
	coretesting.GitSuite
	agenttest.AgentSuite
}

var _ = gc.Suite(&CAASOperatorSuite{})

func (s *CAASOperatorSuite) SetUpSuite(c *gc.C) {
	s.GitSuite.SetUpSuite(c)
	s.AgentSuite.SetUpSuite(c)
}

func (s *CAASOperatorSuite) TearDownSuite(c *gc.C) {
	s.AgentSuite.TearDownSuite(c)
	s.GitSuite.TearDownSuite(c)
}

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.AgentSuite.SetUpTest(c)
}

func (s *CAASOperatorSuite) TearDownTest(c *gc.C) {
	s.AgentSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

// primeAgent creates an application, and sets up the application agent's directory.
// It returns new application and the agent's configuration.
func (s *CAASOperatorSuite) primeAgent(c *gc.C) (*state.Application, agent.Config, *tools.Tools) {
	app := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	err := app.SetPassword(initialApplicationPassword)
	c.Assert(err, jc.ErrorIsNil)
	conf, tools := s.PrimeAgent(c, app.Tag(), initialApplicationPassword)
	return app, conf, tools
}

func (s *CAASOperatorSuite) newAgent(c *gc.C, app *state.Application) *CaasOperatorAgent {
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
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

func (s *CAASOperatorSuite) TestParseSuccess(c *gc.C) {
	s.primeAgent(c)
	// Now init actually reads the agent configuration file.
	// So use the prime agent call which installs a wordpress unit.
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(a, []string{
		"--data-dir", s.DataDir(),
		"--application-name", "wordpress",
		"--log-to-stderr",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(a.AgentConf.DataDir(), gc.Equals, s.DataDir())
	c.Check(a.ApplicationName, gc.Equals, "wordpress")
}

func (s *CAASOperatorSuite) TestParseMissing(c *gc.C) {
	uc, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(uc, []string{
		"--data-dir", "jc",
	})

	c.Assert(err, gc.ErrorMatches, "--application-name option must be set")
}

func (s *CAASOperatorSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{"--application-name", "wordpress/0"},
		{"--application-name", "wordpress/seventeen"},
		{"--application-name", "wordpress/-32"},
		{"--application-name", "wordpress/wild/9"},
		{"--application-name", "20"},
	} {
		a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
		c.Assert(err, jc.ErrorIsNil)

		err = cmdtesting.InitCommand(a, append(args, "--data-dir", "jc"))
		c.Check(err, gc.ErrorMatches, `--application-name option expects "<application>" argument`)
	}
}

func (s *CAASOperatorSuite) TestParseUnknown(c *gc.C) {
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)

	err = cmdtesting.InitCommand(a, []string{
		"--application-name", "wordpress",
		"thundering typhoons",
	})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
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

func (s *CAASOperatorSuite) TestUseLumberjack(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)

	a := CaasOperatorAgent{
		AgentConf:       FakeAgentConfig{},
		ctx:             ctx,
		ApplicationName: "mysql",
	}

	err = a.Init(nil)
	c.Assert(err, gc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsTrue)
	c.Check(l.MaxAge, gc.Equals, 0)
	c.Check(l.MaxBackups, gc.Equals, 2)
	c.Check(l.Filename, gc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, gc.Equals, 300)
}

func (s *CAASOperatorSuite) TestDontUseLumberjack(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)

	a := CaasOperatorAgent{
		AgentConf:       FakeAgentConfig{},
		ctx:             ctx,
		ApplicationName: "mysql",

		// this is what would get set by the CLI flags to tell us not to log to
		// the file.
		logToStdErr: true,
	}

	err = a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

func (s *CAASOperatorSuite) TestWorkers(c *gc.C) {
	coretesting.SkipIfWindowsBug(c, "lp:1610993")
	tracker := NewEngineTracker()
	instrumented := TrackCAASOperator(c, tracker, caasOperatorManifolds)
	s.PatchValue(&caasOperatorManifolds, instrumented)

	app, _, _ := s.primeAgent(c)
	ctx := cmdtesting.Context(c)
	a, err := NewCaasOperatorAgent(ctx, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--application-name", app.Name())

	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	matcher := NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysCAASWorkers, notMigratingCAASWorkers...))
	WaitMatch(c, matcher.Check, coretesting.LongWait, s.BackingState.StartSync)
}
