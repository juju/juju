// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type JujuLogSuite struct {
	relationSuite
}

var _ = gc.Suite(&JujuLogSuite{})

func assertLogs(c *gc.C, ctx jujuc.Context, writer *loggo.TestWriter, unitname, badge string) {
	msg1 := "the chickens"
	msg2 := "are 110% AWESOME"
	com, err := jujuc.NewCommand(ctx, cmdString("juju-log"))
	c.Assert(err, jc.ErrorIsNil)
	for _, t := range []struct {
		args  []string
		level loggo.Level
	}{
		{
			level: loggo.INFO,
		}, {
			args:  []string{"--debug"},
			level: loggo.DEBUG,
		}, {
			args:  []string{"--log-level", "TRACE"},
			level: loggo.TRACE,
		}, {
			args:  []string{"--log-level", "info"},
			level: loggo.INFO,
		}, {
			args:  []string{"--log-level", "WaRnInG"},
			level: loggo.WARNING,
		}, {
			args:  []string{"--log-level", "error"},
			level: loggo.ERROR,
		},
	} {
		writer.Clear()
		c.Assert(err, jc.ErrorIsNil)

		args := append(t.args, msg1, msg2)
		code := cmd.Main(com, &cmd.Context{}, args)
		c.Assert(code, gc.Equals, 0)
		log := writer.Log()
		c.Assert(log, gc.HasLen, 1)
		c.Assert(log[0].Level, gc.Equals, t.level)
		c.Assert(log[0].Module, gc.Equals, fmt.Sprintf("unit.%s.juju-log", unitname))
		c.Assert(log[0].Message, gc.Equals, fmt.Sprintf("%s%s %s", badge, msg1, msg2))
	}
}

func (s *JujuLogSuite) TestBadges(c *gc.C) {
	tw := &loggo.TestWriter{}
	_, err := loggo.ReplaceDefaultWriter(tw)
	loggo.GetLogger("unit").SetLogLevel(loggo.TRACE)
	c.Assert(err, jc.ErrorIsNil)
	hctx, _ := s.newHookContext(-1, "")
	assertLogs(c, hctx, tw, "u/0", "")
	hctx, _ = s.newHookContext(1, "u/1")
	assertLogs(c, hctx, tw, "u/0", "peer1:1: ")
}

func (s *JujuLogSuite) newJujuLogCommand(c *gc.C) cmd.Command {
	ctx, _ := s.newHookContext(-1, "")
	com, err := jujuc.NewCommand(ctx, cmdString("juju-log"))
	c.Assert(err, jc.ErrorIsNil)
	return com
}

func (s *JujuLogSuite) TestRequiresMessage(c *gc.C) {
	com := s.newJujuLogCommand(c)
	testing.TestInit(c, com, nil, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *gc.C) {
	com := s.newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l"}, "flag needs an argument.*")

	com = s.newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"--log-level"}, "flag needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *gc.C) {
	com := s.newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"-l", "FATAL"}, "no message specified")

	com = s.newJujuLogCommand(c)
	testing.TestInit(c, com, []string{"--log-level", "FATAL"}, "no message specified")
}

func (s *JujuLogSuite) TestLogDeprecation(c *gc.C) {
	com := s.newJujuLogCommand(c)
	ctx, err := testing.RunCommand(c, com, "--format", "foo", "msg")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "--format flag deprecated for command \"juju-log\"")
}
