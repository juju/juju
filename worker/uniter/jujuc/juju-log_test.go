// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"launchpad.net/loggo"
)

type JujuLogSuite struct {
	ContextSuite
}

var _ = gc.Suite(&JujuLogSuite{})

var commonLogTests = []struct {
	debugFlag bool
	level     loggo.Level
}{
	{false, loggo.INFO},
	{true, loggo.DEBUG},
}

func assertLogs(c *gc.C, ctx jujuc.Context, badge string) {
	loggo.ConfigureLoggers("juju=DEBUG")
	writer := &loggo.TestWriter{}
	old_writer, err := loggo.ReplaceDefaultWriter(writer)
	c.Assert(err, gc.IsNil)
	defer loggo.ReplaceDefaultWriter(old_writer)
	msg1 := "the chickens"
	msg2 := "are 110% AWESOME"
	com, err := jujuc.NewCommand(ctx, "juju-log")
	c.Assert(err, gc.IsNil)
	for _, t := range commonLogTests {
		writer.Clear()
		c.Assert(err, gc.IsNil)

		var args []string
		if t.debugFlag {
			args = []string{"--debug", msg1, msg2}
		} else {
			args = []string{msg1, msg2}
		}
		code := cmd.Main(com, &cmd.Context{}, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(writer.Log, gc.HasLen, 1)
		c.Assert(writer.Log[0].Level, gc.Equals, t.level)
		c.Assert(writer.Log[0].Message, gc.Equals, fmt.Sprintf("%s: %s %s", badge, msg1, msg2))
	}
}

func (s *JujuLogSuite) TestBadges(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	assertLogs(c, hctx, "u/0")
	hctx = s.GetHookContext(c, 1, "u/1")
	assertLogs(c, hctx, "u/0 peer1:1")
}

func newJujuLogCommand(c *gc.C) cmd.Command {
	ctx := &Context{}
	com, err := jujuc.NewCommand(ctx, "juju-log")
	c.Assert(err, gc.IsNil)
	return com
}

func (s *JujuLogSuite) TestRequiresMessage(c *gc.C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, nil, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *gc.C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l"}, "flag needs an argument.*")

	com = newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"--log-level"}, "flag needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *gc.C) {
	com := newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l", "FATAL"}, "no message specified")

	com = newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"--log-level", "FATAL"}, "no message specified")
}

func (s *JujuLogSuite) TestLogDeprecation(c *gc.C) {
	com := newJujuLogCommand(c)
	ctx, err := testing.RunCommand(c, com, []string{"--format", "foo", "msg"})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "--format flag deprecated for command \"juju-log\"")
}
