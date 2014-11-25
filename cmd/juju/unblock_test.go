// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
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

func (s *UnblockCommandSuite) runUnblockTestAndCompare(c *gc.C, operation string, expectedValue bool) {
	err := runUnblockCommand(c, operation)
	c.Assert(err, jc.ErrorIsNil)

	expectedOp := config.BlockKeyPrefix + strings.ToLower(operation)
	expectedCfg := map[string]interface{}{expectedOp: expectedValue}
	c.Assert(s.mockClient.cfg, gc.DeepEquals, expectedCfg)
}

func (s *UnblockCommandSuite) TestUnblockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c), `.*must specify one of.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "destroy-environment", "change"), `.*must specify one of.*`)
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
