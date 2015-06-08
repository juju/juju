// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process/context"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type commandSuite struct {
	baseSuite

	cmdName string
	cmd     cmd.Command
	cmdCtx  *cmd.Context
}

func (s *commandSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.cmdCtx = coretesting.Context(c)
}

func (s *commandSuite) checkStdout(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkStderr(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stderr.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkCommandRegistered(c *gc.C) {
	// TODO(ericsnow) finish!
	panic("not finished")
	jujuc.NewCommand(s.Hctx, s.cmdName)
}

func (s *commandSuite) checkHelp(c *gc.C, expected string) {
	code := cmd.Main(s.cmd, s.cmdCtx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)

	s.checkStdout(c, expected)
}

func (s *commandSuite) checkRun(c *gc.C, expectedOut, expectedErr string) {
	context.SetComponent(s.cmd, jujuctesting.ContextComponent{s.Stub})

	err := s.cmd.Run(s.cmdCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkStdout(c, expectedOut)
	s.checkStderr(c, expectedErr)
}
