// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type UnblockCommandSuite struct {
	ProtectionCommandSuite
}

var _ = gc.Suite(&UnblockCommandSuite{})

func runUnblockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&UnblockCommand{}), args...)
	return err
}

func (s *UnblockCommandSuite) runUnblockTestAndAssert(c *gc.C, operation string, expectedVarValue gc.Checker) {
	err := runUnblockCommand(c, operation)
	c.Assert(err, gc.IsNil)
	s.assertEnvVariableSet(c, operation, expectedVarValue)
}

func (s *UnblockCommandSuite) TestUnblockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c), `.*specify operation.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "destroy-environment", "change"), `.*specify operation.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdOperationWithSeparator(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "destroy-environment|"), `.*valid argument.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdUnknownJujuOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "add-machine"), `.*valid argument.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdUnknownOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "blah"), `.*valid argument.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdValidDestroyEnvOperationUpperCase(c *gc.C) {
	s.runUnblockTestAndAssert(c, "DESTROY-ENVIRONMENT", jc.IsFalse)
}

func (s *UnblockCommandSuite) TestUnblockCmdValidDestroyEnvOperation(c *gc.C) {
	s.runUnblockTestAndAssert(c, "destroy-environment", jc.IsFalse)
}

func (s *UnblockCommandSuite) TestUnblockCmdEnvironmentArg(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = runUnblockCommand(c, "destroy-environment", "-e", "dummyenv")
	c.Assert(err, gc.IsNil)
}
