// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
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

func (s *commandSuite) setCommand(c *gc.C, name string, cmd cmd.Command) {
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.ResetCalls()

	s.cmdName = name + jujuc.CmdSuffix
	s.cmd = cmd
}

func (s *commandSuite) checkStdout(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkStderr(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stderr.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkDetails(c *gc.C, expected plugin.ProcDetails) {
	info := context.GetCmdInfo(s.cmd)
	c.Check(info.Details, jc.DeepEquals, expected)
}

func (s *commandSuite) checkStatus(c *gc.C, expected process.Status) {
	info := context.GetCmdInfo(s.cmd)
	c.Check(info.Status, jc.DeepEquals, expected)
}

func (s *commandSuite) checkCommandRegistered(c *gc.C) {
	component := &context.Context{}
	hctx, info := s.NewHookContext()
	info.SetComponent("process", component)

	cmd, err := jujuc.NewCommand(hctx.Context, s.cmdName)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cmd, gc.NotNil)
}

func (s *commandSuite) checkHelp(c *gc.C, expected string) {
	code := cmd.Main(s.cmd, s.cmdCtx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)

	s.checkStdout(c, expected)
}

func (s *commandSuite) checkRun(c *gc.C, expectedOut, expectedErr string) {
	context.SetComponent(s.cmd, newStubContextComponent(s.Stub))

	err := s.cmd.Run(s.cmdCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkStdout(c, expectedOut)
	s.checkStderr(c, expectedErr)
}
