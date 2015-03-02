// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/testing"
)

type UnblockCommandSuite struct {
	ProtectionCommandSuite
	mockClient *block.MockBlockClient
}

func (s *UnblockCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockClient = &block.MockBlockClient{}
	s.PatchValue(block.UnblockClient, func(p *block.UnblockCommand) (block.UnblockClientAPI, error) {
		return s.mockClient, nil
	})
}

var _ = gc.Suite(&UnblockCommandSuite{})

func runUnblockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&block.UnblockCommand{}), args...)
	return err
}

func (s *UnblockCommandSuite) assertRunUnblock(c *gc.C, operation string) {
	err := runUnblockCommand(c, operation)
	c.Assert(err, jc.ErrorIsNil)

	expectedOp := block.TypeFromOperation(strings.ToLower(operation))
	c.Assert(s.mockClient.BlockType, gc.DeepEquals, expectedOp)
}

func (s *UnblockCommandSuite) TestUnblockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c), `.*must specify one of.*`)
}

func (s *UnblockCommandSuite) TestUnblockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runUnblockCommand(c, "destroy-environment", "change"), `.*can only specify block type.*`)
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
	s.assertRunUnblock(c, "DESTROY-ENVIRONMENT")
}

func (s *UnblockCommandSuite) TestUnblockCmdValidDestroyEnvOperation(c *gc.C) {
	s.assertRunUnblock(c, "destroy-environment")
}
