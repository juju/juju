// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/natefinch/lumberjack.v2"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/logsender"
)

type CAASOperatorSuite struct {
	coretesting.BaseSuite

	rootDir string
}

var _ = gc.Suite(&CAASOperatorSuite{})

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.rootDir = c.MkDir()
}

func (s *CAASOperatorSuite) dataDir() string {
	return filepath.Join(s.rootDir, "/var/lib/juju")
}

func (s *CAASOperatorSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*gc.C) { logger.Close() })
	return logger
}

func (s *CAASOperatorSuite) TestParseSuccess(c *gc.C) {
	// Now init actually reads the agent configuration file.
	a, err := NewCaasOperatorAgent(nil, s.newBufferedLogWriter())
	c.Assert(err, jc.ErrorIsNil)
	err = cmdtesting.InitCommand(a, []string{
		"--data-dir", s.dataDir(),
		"--application-name", "wordpress",
		"--log-to-stderr",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(a.AgentConf.DataDir(), gc.Equals, s.dataDir())
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
