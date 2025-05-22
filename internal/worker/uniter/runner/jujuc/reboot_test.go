// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"testing"

	"github.com/juju/gnuflag"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type JujuRebootSuite struct {
	ContextSuite
}

func TestJujuRebootSuite(t *testing.T) {
	tc.Run(t, &JujuRebootSuite{})
}

func (s *JujuRebootSuite) TestNewJujuRebootCommand(c *tc.C) {
	cmd, err := jujuc.NewJujuRebootCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmd, tc.DeepEquals, &jujuc.JujuRebootCommand{})
}

func (s *JujuRebootSuite) TestInfo(c *tc.C) {
	rebootCmd, err := jujuc.NewJujuRebootCommand(nil)
	c.Assert(err, tc.ErrorIsNil)
	cmdInfo := rebootCmd.Info()

	c.Assert(cmdInfo.Name, tc.Equals, "juju-reboot")
	c.Assert(cmdInfo.Args, tc.Equals, "")
	c.Assert(cmdInfo.Purpose, tc.Equals, "Reboot the host machine.")
}

func (s *JujuRebootSuite) TestSetFlags(c *tc.C) {
	rebootCmd := jujuc.JujuRebootCommand{Now: true}
	fs := &gnuflag.FlagSet{}

	rebootCmd.SetFlags(fs)

	flag := fs.Lookup("now")
	c.Assert(flag, tc.NotNil)
}

func (s *JujuRebootSuite) TestJujuRebootCommand(c *tc.C) {
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
		com, err := jujuc.NewCommand(hctx, "juju-reboot")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, tc.Equals, t.code)
		c.Check(hctx.rebootPriority, tc.Equals, t.priority)
	}
}

func (s *JujuRebootSuite) TestRebootInActions(c *tc.C) {
	jujucCtx := &actionGetContext{}
	com, err := jujuc.NewCommand(jujucCtx, "juju-reboot")
	c.Assert(err, tc.ErrorIsNil)
	cmdCtx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), cmdCtx, nil)
	c.Check(code, tc.Equals, 1)
	c.Assert(cmdtesting.Stderr(cmdCtx), tc.Equals, "ERROR juju-reboot is not supported when running an action.\n")
}
