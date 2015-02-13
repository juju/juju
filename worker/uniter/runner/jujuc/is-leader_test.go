// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type IsLeaderSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&IsLeaderSuite{})

func (s *IsLeaderSuite) TestInitSuccess(c *gc.C) {
	command := jujuc.NewIsLeaderCommand(nil)
	err := command.Init(nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *IsLeaderSuite) TestInitError(c *gc.C) {
	command := jujuc.NewIsLeaderCommand(nil)
	err := command.Init([]string{"blah"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blah"\]`)
}

func (s *IsLeaderSuite) TestIsLeaderError(c *gc.C) {
	command := jujuc.NewIsLeaderCommand(isLeaderContext{
		err: errors.New("pow"),
	})
	context := testing.Context(c)
	code := cmd.Main(command, context, nil)
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(context.Stdout), gc.Equals, "")
	c.Check(bufferString(context.Stderr), gc.Equals, "error: leadership status unknown: pow\n")
}

func (s *IsLeaderSuite) TestFormatError(c *gc.C) {
	command := jujuc.NewIsLeaderCommand(nil)
	context := testing.Context(c)
	code := cmd.Main(command, context, []string{"--format", "bad"})
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(context.Stdout), gc.Equals, "")
	c.Check(bufferString(context.Stderr), gc.Equals, `error: invalid value "bad" for flag --format: unknown format "bad"`+"\n")
}

func testOutput(c *gc.C, leader bool, args []string, expect string) {
	command := jujuc.NewIsLeaderCommand(isLeaderContext{
		leader: leader,
	})
	context := testing.Context(c)
	code := cmd.Main(command, context, args)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(context.Stdout), gc.Equals, expect)
	c.Check(bufferString(context.Stderr), gc.Equals, "")
}

func (s *IsLeaderSuite) TestFormatDefaultYes(c *gc.C) {
	testOutput(c, true, nil, "True\n")
}

func (s *IsLeaderSuite) TestFormatDefaultNo(c *gc.C) {
	testOutput(c, false, nil, "False\n")
}

func (s *IsLeaderSuite) TestFormatSmartYes(c *gc.C) {
	testOutput(c, true, []string{"--format", "smart"}, "True\n")
}

func (s *IsLeaderSuite) TestFormatSmartNo(c *gc.C) {
	testOutput(c, false, []string{"--format", "smart"}, "False\n")
}

func testParseOutput(c *gc.C, leader bool, args []string, checker gc.Checker) {
	command := jujuc.NewIsLeaderCommand(isLeaderContext{
		leader: leader,
	})
	context := testing.Context(c)
	code := cmd.Main(command, context, args)
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(context.Stdout), checker, leader)
	c.Check(bufferString(context.Stderr), gc.Equals, "")
}

func (s *IsLeaderSuite) TestFormatYamlYes(c *gc.C) {
	testParseOutput(c, true, []string{"--format", "yaml"}, jc.YAMLEquals)
}

func (s *IsLeaderSuite) TestFormatYamlNo(c *gc.C) {
	testParseOutput(c, false, []string{"--format", "yaml"}, jc.YAMLEquals)
}

func (s *IsLeaderSuite) TestFormatJsonYes(c *gc.C) {
	testParseOutput(c, true, []string{"--format", "json"}, jc.JSONEquals)
}

func (s *IsLeaderSuite) TestFormatJsonNo(c *gc.C) {
	testParseOutput(c, false, []string{"--format", "json"}, jc.JSONEquals)
}

type isLeaderContext struct {
	jujuc.Context
	leader bool
	err    error
}

func (ctx isLeaderContext) IsLeader() (bool, error) {
	return ctx.leader, ctx.err
}
