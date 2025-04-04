// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type JujuRebootSuite struct {
	ContextSuite
}

var _ = gc.Suite(&JujuRebootSuite{})

func (s *JujuRebootSuite) TestNewJujuRebootCommand(c *gc.C) {
	cmd, err := jujuc.NewJujuRebootCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd, gc.DeepEquals, &jujuc.JujuRebootCommand{})
}

func (s *JujuRebootSuite) TestInfo(c *gc.C) {
	rebootCmd, err := jujuc.NewJujuRebootCommand(nil)
	c.Assert(err, jc.ErrorIsNil)
	cmdInfo := rebootCmd.Info()

	c.Assert(cmdInfo.Name, gc.Equals, "juju-reboot")
	c.Assert(cmdInfo.Args, gc.Equals, "")
	c.Assert(cmdInfo.Purpose, gc.Equals, "Reboot the host machine.")
}

func (s *JujuRebootSuite) TestSetFlags(c *gc.C) {
	rebootCmd := jujuc.JujuRebootCommand{Now: true}
	fs := &gnuflag.FlagSet{}

	rebootCmd.SetFlags(fs)

	flag := fs.Lookup("now")
	c.Assert(flag, gc.NotNil)
}

func (s *JujuRebootSuite) TestJujuRebootCommand(c *gc.C) {
	var jujuRebootTests = []struct {
		summary  string
		hctx     *Context
		args     []string
		code     int
		priority jujuc.RebootPriority
	}{{
		summary:  "test reboot priority defaulting to RebootAfterHook",
		hctx:     &Context{shouldError: false, rebootPriority: jujuc.RebootSkip},
		args:     []string{},
		code:     0,
		priority: jujuc.RebootAfterHook,
	}, {
		summary:  "test reboot priority being set to RebootNow",
		hctx:     &Context{shouldError: false, rebootPriority: jujuc.RebootSkip},
		args:     []string{"--now"},
		code:     0,
		priority: jujuc.RebootNow,
	}, {
		summary:  "test a failed running of juju-reboot",
		hctx:     &Context{shouldError: true, rebootPriority: jujuc.RebootSkip},
		args:     []string{},
		code:     1,
		priority: jujuc.RebootAfterHook,
	}, {
		summary:  "test a failed running with parameter provided",
		hctx:     &Context{shouldError: true, rebootPriority: jujuc.RebootSkip},
		args:     []string{"--now"},
		code:     1,
		priority: jujuc.RebootNow,
	}, {
		summary:  "test invalid args provided",
		hctx:     &Context{shouldError: false, rebootPriority: jujuc.RebootSkip},
		args:     []string{"--way", "--too", "--many", "--args"},
		code:     2,
		priority: jujuc.RebootSkip,
	}}

	for i, t := range jujuRebootTests {
		c.Logf("Test %d: %s", i, t.summary)

		hctx := s.newHookContext(c)
		hctx.shouldError = t.hctx.shouldError
		hctx.rebootPriority = t.hctx.rebootPriority
		com, err := jujuc.NewHookCommand(hctx, "juju-reboot")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		c.Check(hctx.rebootPriority, gc.Equals, t.priority)
	}
}

func (s *JujuRebootSuite) TestRebootInActions(c *gc.C) {
	jujucCtx := &actionGetContext{}
	com, err := jujuc.NewHookCommand(jujucCtx, "juju-reboot")
	c.Assert(err, jc.ErrorIsNil)
	cmdCtx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), cmdCtx, nil)
	c.Check(code, gc.Equals, 1)
	c.Assert(cmdtesting.Stderr(cmdCtx), gc.Equals, "ERROR juju-reboot is not supported when running an action.\n")
}
