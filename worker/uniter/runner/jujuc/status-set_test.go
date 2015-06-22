// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type statusSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&statusSetSuite{})

func (s *statusSetSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
}

var statusSetInitTests = []struct {
	args []string
	err  string
}{
	{[]string{"maintenance"}, ""},
	{[]string{"maintenance", ""}, ""},
	{[]string{"maintenance", "hello"}, ""},
	{[]string{}, `invalid args, require <status> \[message\]`},
	{[]string{"maintenance", "hello", "extra"}, `unrecognized args: \["extra"\]`},
	{[]string{"foo", "hello"}, `invalid status "foo", expected one of \[maintenance blocked waiting active\]`},
}

func (s *statusSetSuite) TestStatusSetInit(c *gc.C) {
	for i, t := range statusSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		testing.TestInit(c, com, t.args, t.err)
	}
}

func (s *statusSetSuite) TestHelp(c *gc.C) {
	hctx := s.GetStatusHookContext(c)
	com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := "" +
		"usage: status-set [options] <maintenance | blocked | waiting | active> [message]\n" +
		"purpose: set status information\n" +
		"\n" +
		"options:\n" +
		"--service  (= false)\n" +
		"    set this status for the service to which the unit belongs if the unit is the leader\n" +
		"\n" +
		"Sets the workload status of the charm. Message is optional.\n" +
		"The \"last updated\" attribute of the status is set, even if the\n" +
		"status and message are the same as what's already set.\n"

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *statusSetSuite) TestStatus(c *gc.C) {
	for i, args := range [][]string{
		[]string{"maintenance", "doing some work"},
		[]string{"active", ""},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.UnitStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Status, gc.Equals, args[0])
		c.Assert(status.Info, gc.Equals, args[1])
	}
}

func (s *statusSetSuite) TestServiceStatus(c *gc.C) {
	for i, args := range [][]string{
		[]string{"--service", "maintenance", "doing some work"},
		[]string{"--service", "active", ""},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, cmdString("status-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.ServiceStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Service.Status, gc.Equals, args[1])
		c.Assert(status.Service.Info, gc.Equals, args[2])
		c.Assert(status.Units, jc.DeepEquals, []jujuc.StatusInfo{})

	}
}
