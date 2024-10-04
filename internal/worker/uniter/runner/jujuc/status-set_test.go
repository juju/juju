// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"context"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
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
		com, err := jujuc.NewCommand(hctx, "status-set")
		c.Assert(err, jc.ErrorIsNil)
		cmdtesting.TestInit(c, com, t.args, t.err)
	}
}

func (s *statusSetSuite) TestStatus(c *gc.C) {
	for i, args := range [][]string{
		{"maintenance", "doing some work"},
		{"active", ""},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, "status-set")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.UnitStatus(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Status, gc.Equals, args[0])
		c.Assert(status.Info, gc.Equals, args[1])
	}
}

func (s *statusSetSuite) TestApplicationStatus(c *gc.C) {
	for i, args := range [][]string{
		{"--application", "maintenance", "doing some work"},
		{"--application", "active", ""},
	} {
		c.Logf("test %d: %#v", i, args)
		hctx := s.GetStatusHookContext(c)
		com, err := jujuc.NewCommand(hctx, "status-set")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		status, err := hctx.ApplicationStatus(context.Background())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Application.Status, gc.Equals, args[1])
		c.Assert(status.Application.Info, gc.Equals, args[2])
		c.Assert(status.Units, jc.DeepEquals, []jujuc.StatusInfo{})

	}
}
