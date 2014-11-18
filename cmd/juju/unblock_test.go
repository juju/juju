// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
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

func (s *UnblockCommandSuite) runUnblockTestAndCompare(c *gc.C, operation string, expectedVarValue bool) {
	s.mockClient.operation = operation
	s.mockClient.expectedVarValue = expectedVarValue
	err := runUnblockCommand(c, operation)
	c.Assert(err, gc.IsNil)
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
	s.runUnblockTestAndCompare(c, "DESTROY-ENVIRONMENT", false)
}

func (s *UnblockCommandSuite) TestUnblockCmdValidDestroyEnvOperation(c *gc.C) {
	s.runUnblockTestAndCompare(c, "destroy-environment", false)
}
