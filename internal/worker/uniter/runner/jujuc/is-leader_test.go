// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type isLeaderSuite struct {
	testing.BaseSuite
}

func TestIsLeaderSuite(t *stdtesting.T) {
	tc.Run(t, &isLeaderSuite{})
}

func (s *isLeaderSuite) TestInitError(c *tc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = command.Init([]string{"blah"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["blah"\]`)
}

func (s *isLeaderSuite) TestInitSuccess(c *tc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = command.Init(nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *isLeaderSuite) TestFormatError(c *tc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"--format", "bad"})
	c.Check(code, tc.Equals, 2)
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, `ERROR invalid value "bad" for option --format: unknown format "bad"`+"\n")
}

func (s *isLeaderSuite) TestIsLeaderError(c *tc.C) {
	jujucContext := &isLeaderContext{err: errors.New("pow")}
	command, err := jujuc.NewIsLeaderCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, nil)
	c.Check(code, tc.Equals, 1)
	c.Check(jujucContext.called, tc.IsTrue)
	c.Check(bufferString(runContext.Stdout), tc.Equals, "")
	c.Check(bufferString(runContext.Stderr), tc.Equals, "ERROR leadership status unknown: pow\n")
}

func (s *isLeaderSuite) TestFormatDefaultYes(c *tc.C) {
	s.testOutput(c, true, nil, "True\n")
}

func (s *isLeaderSuite) TestFormatDefaultNo(c *tc.C) {
	s.testOutput(c, false, nil, "False\n")
}

func (s *isLeaderSuite) TestFormatSmartYes(c *tc.C) {
	s.testOutput(c, true, []string{"--format", "smart"}, "True\n")
}

func (s *isLeaderSuite) TestFormatSmartNo(c *tc.C) {
	s.testOutput(c, false, []string{"--format", "smart"}, "False\n")
}

func (s *isLeaderSuite) TestFormatYamlYes(c *tc.C) {
	s.testParseOutput(c, true, []string{"--format", "yaml"}, tc.YAMLEquals)
}

func (s *isLeaderSuite) TestFormatYamlNo(c *tc.C) {
	s.testParseOutput(c, false, []string{"--format", "yaml"}, tc.YAMLEquals)
}

func (s *isLeaderSuite) TestFormatJsonYes(c *tc.C) {
	s.testParseOutput(c, true, []string{"--format", "json"}, tc.JSONEquals)
}

func (s *isLeaderSuite) TestFormatJsonNo(c *tc.C) {
	s.testParseOutput(c, false, []string{"--format", "json"}, tc.JSONEquals)
}

func (s *isLeaderSuite) testOutput(c *tc.C, leader bool, args []string, expect string) {
	jujucContext := &isLeaderContext{leader: leader}
	command, err := jujuc.NewIsLeaderCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, args)
	c.Check(code, tc.Equals, 0)
	c.Check(jujucContext.called, tc.IsTrue)
	c.Check(bufferString(runContext.Stdout), tc.Equals, expect)
	c.Check(bufferString(runContext.Stderr), tc.Equals, "")
}

func (s *isLeaderSuite) testParseOutput(c *tc.C, leader bool, args []string, checker tc.Checker) {
	jujucContext := &isLeaderContext{leader: leader}
	command, err := jujuc.NewIsLeaderCommand(jujucContext)
	c.Assert(err, tc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, args)
	c.Check(code, tc.Equals, 0)
	c.Check(jujucContext.called, tc.IsTrue)
	c.Check(bufferString(runContext.Stdout), checker, leader)
	c.Check(bufferString(runContext.Stderr), tc.Equals, "")
}

type isLeaderContext struct {
	jujuc.Context
	called bool
	leader bool
	err    error
}

func (ctx *isLeaderContext) IsLeader() (bool, error) {
	ctx.called = true
	return ctx.leader, ctx.err
}
