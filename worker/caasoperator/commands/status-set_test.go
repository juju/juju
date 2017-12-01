// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator/commands"
)

type StatusSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&StatusSetSuite{})

func (s *StatusSetSuite) SetUpTest(c *gc.C) {
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

func (s *StatusSetSuite) TestStatusSetInit(c *gc.C) {
	for i, t := range statusSetInitTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetStatusContext(c)
		com, err := commands.NewCommand(hctx, "status-set")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, com, t.args, t.err)
	}
}

func (s *StatusSetSuite) TestHelp(c *gc.C) {
	hctx := s.GetStatusContext(c)
	com, err := commands.NewCommand(hctx, "status-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := "" +
		"Usage: status-set <maintenance | blocked | waiting | active> [message]\n" +
		"\n" +
		"Summary:\n" +
		"set status information\n" +
		"\n" +
		"Details:\n" +
		"Sets the workload status of the charm. Message is optional.\n" +
		"The \"last updated\" attribute of the status is set, even if the\n" +
		"status and message are the same as what's already set.\n"

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *StatusSetSuite) TestStatus(c *gc.C) {
	for i, args := range [][]string{
		{"maintenance", "doing some work"},
		{"active", ""},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusContext(c)
		com, err := commands.NewCommand(hctx, "status-set")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(com, ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.ApplicationStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Status, gc.Equals, args[0])
		c.Assert(status.Info, gc.Equals, args[1])
	}
}
