// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

type commandSuite struct {
	baseSuite

	cmdName string
	cmd     cmd.Command
	cmdCtx  *cmd.Context
	compCtx *stubContextComponent
	Ctx     *stubHookContext
}

func (s *commandSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.compCtx = newStubContextComponent(s.Stub)
	hctx, info := s.NewHookContext()
	info.SetComponent(workload.ComponentName, s.compCtx)
	s.Ctx = hctx
	s.cmdCtx = coretesting.Context(c)

	s.setMetadata()
}

func (s *commandSuite) readDefinitions(ctx *cmd.Context) (map[string]charm.Workload, error) {
	return s.compCtx.definitions, nil
}

func (s *commandSuite) setCommand(c *gc.C, name string, cmd cmd.Command) {
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.ResetCalls()

	s.cmdName = name + jujuc.CmdSuffix
	s.cmd = cmd
}

func (s *commandSuite) setMetadata(workloads ...workload.Info) {
	for _, wl := range workloads {
		definition := wl.Workload
		s.compCtx.definitions[definition.Name] = definition
	}
}

func (s *commandSuite) checkStdout(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stdout.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkStderr(c *gc.C, expected string) {
	c.Check(s.cmdCtx.Stderr.(*bytes.Buffer).String(), gc.Equals, expected)
}

func (s *commandSuite) checkCommandRegistered(c *gc.C) {
	component := &context.Context{}
	hctx, info := s.NewHookContext()
	info.SetComponent(workload.ComponentName, component)

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
	err := s.cmd.Run(s.cmdCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkStdout(c, expectedOut)
	s.checkStderr(c, expectedErr)
}
