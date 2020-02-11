// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type isLeaderSuite struct {
	jujucSuite
}

var _ = gc.Suite(&isLeaderSuite{})

func (s *isLeaderSuite) TestInitError(c *gc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = command.Init([]string{"blah"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blah"\]`)
}

func (s *isLeaderSuite) TestInitSuccess(c *gc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = command.Init(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *isLeaderSuite) TestFormatError(c *gc.C) {
	command, err := jujuc.NewIsLeaderCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, []string{"--format", "bad"})
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, `ERROR invalid value "bad" for option --format: unknown format "bad"`+"\n")
}

func (s *isLeaderSuite) TestIsLeaderError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectLeadershipError(errors.New("pow"))
	command, err := jujuc.NewIsLeaderCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, nil)
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(runContext.Stdout), gc.Equals, "")
	c.Check(bufferString(runContext.Stderr), gc.Equals, "ERROR leadership status unknown: pow\n")
}

func (s *isLeaderSuite) TestFormatDefaultYes(c *gc.C) {
	s.testOutput(c, true, nil, "True\n")
}

func (s *isLeaderSuite) TestFormatDefaultNo(c *gc.C) {
	s.testOutput(c, false, nil, "False\n")
}

func (s *isLeaderSuite) TestFormatSmartYes(c *gc.C) {
	s.testOutput(c, true, []string{"--format", "smart"}, "True\n")
}

func (s *isLeaderSuite) TestFormatSmartNo(c *gc.C) {
	s.testOutput(c, false, []string{"--format", "smart"}, "False\n")
}

func (s *isLeaderSuite) TestFormatYamlYes(c *gc.C) {
	s.testParseOutput(c, true, []string{"--format", "yaml"}, jc.YAMLEquals)
}

func (s *isLeaderSuite) TestFormatYamlNo(c *gc.C) {
	s.testParseOutput(c, false, []string{"--format", "yaml"}, jc.YAMLEquals)
}

func (s *isLeaderSuite) TestFormatJsonYes(c *gc.C) {
	s.testParseOutput(c, true, []string{"--format", "json"}, jc.JSONEquals)
}

func (s *isLeaderSuite) TestFormatJsonNo(c *gc.C) {
	s.testParseOutput(c, false, []string{"--format", "json"}, jc.JSONEquals)
}

func (s *isLeaderSuite) testOutput(c *gc.C, leader bool, args []string, expect string) {
	defer s.setupMocks(c).Finish()
	s.expectLeadership(leader)
	command, err := jujuc.NewIsLeaderCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, args)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(runContext.Stdout), gc.Equals, expect)
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *isLeaderSuite) testParseOutput(c *gc.C, leader bool, args []string, checker gc.Checker) {
	defer s.setupMocks(c).Finish()
	s.expectLeadership(leader)
	command, err := jujuc.NewIsLeaderCommand(s.mockContext)
	c.Assert(err, jc.ErrorIsNil)
	runContext := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(command), runContext, args)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(runContext.Stdout), checker, leader)
	c.Check(bufferString(runContext.Stderr), gc.Equals, "")
}

func (s *isLeaderSuite) expectLeadership(leader bool) {
	s.mockContext.EXPECT().IsLeader().Return(leader, nil)
}

func (s *isLeaderSuite) expectLeadershipError(err error) {
	s.mockContext.EXPECT().IsLeader().Return(false, err)
}
