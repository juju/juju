// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"launchpad.net/goyaml"

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
	{[]string{}, `invalid args, require <status> \[message\] \[data\]`},
	{[]string{"maintenance", "hello", "{number: 22, string: some string}", "extra"}, `unrecognized args: \["extra"\]`},
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
		"usage: status-set [options] <maintenance | blocked | waiting | active> [message] [data]\n" +
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
	expectedYaml := map[int]string{2: "" +
		"number: \"22\"\n" +
		"string: some string\n",
	}
	for i, args := range [][]string{
		[]string{"maintenance", "doing some work"},
		[]string{"active", ""},
		[]string{"maintenance", "valid data", `{number: 22, string: some string}`},
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
		if len(args) == 3 {
			text, err := goyaml.Marshal(status.Data)
			c.Check(err, jc.ErrorIsNil)
			expected, ok := expectedYaml[i]
			c.Check(ok, jc.IsTrue)
			c.Assert(string(text), gc.Equals, expected)
		}
	}
}

func (s *statusSetSuite) TestServiceStatus(c *gc.C) {
	expectedYaml := map[int]string{2: "" +
		"number: \"22\"\n" +
		"string: some string\n",
	}
	for i, args := range [][]string{
		[]string{"--service", "maintenance", "doing some work"},
		[]string{"--service", "active", ""},
		[]string{"--service", "maintenance", "valid data", `{number: 22, string: some string}`},
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
		if len(args) == 4 {
			text, err := goyaml.Marshal(status.Service.Data)
			c.Check(err, jc.ErrorIsNil)
			expected, ok := expectedYaml[i]
			c.Check(ok, jc.IsTrue)
			c.Assert(string(text), gc.Equals, expected)
		}
		c.Assert(status.Units, jc.DeepEquals, []jujuc.StatusInfo{})

	}
}
