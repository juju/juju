// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/testing"
)

type BlockCommandSuite struct {
	ProtectionCommandSuite
	mockClient *block.MockBlockClient
}

func (s *BlockCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.mockClient = &block.MockBlockClient{}
	s.PatchValue(block.BlockClient, func(p *block.BaseBlockCommand) (block.BlockClientAPI, error) {
		return s.mockClient, nil
	})
}

var _ = gc.Suite(&BlockCommandSuite{})

func (s *BlockCommandSuite) assertBlock(c *gc.C, operation, message string) {
	expectedOp := block.TypeFromOperation(operation)
	c.Assert(s.mockClient.BlockType, gc.DeepEquals, expectedOp)
	c.Assert(s.mockClient.Msg, gc.DeepEquals, message)
}

func (s *BlockCommandSuite) TestBlockCmdMoreArgs(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&block.DestroyCommand{}), "change", "too much")
	c.Assert(
		err,
		gc.ErrorMatches,
		`.*can only specify block message.*`)
}

func (s *BlockCommandSuite) TestBlockCmdNoMessage(c *gc.C) {
	command := block.DestroyCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(&command))
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlock(c, command.Info().Name, "")
}

func (s *BlockCommandSuite) TestBlockDestroyOperations(c *gc.C) {
	command := block.DestroyCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(&command), "TestBlockDestroyOperations")
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlock(c, command.Info().Name, "TestBlockDestroyOperations")
}

func (s *BlockCommandSuite) TestBlockRemoveOperations(c *gc.C) {
	command := block.RemoveCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(&command), "TestBlockRemoveOperations")
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlock(c, command.Info().Name, "TestBlockRemoveOperations")
}

func (s *BlockCommandSuite) TestBlockChangeOperations(c *gc.C) {
	command := block.ChangeCommand{}
	_, err := testing.RunCommand(c, envcmd.Wrap(&command), "TestBlockChangeOperations")
	c.Assert(err, jc.ErrorIsNil)
	s.assertBlock(c, command.Info().Name, "TestBlockChangeOperations")
}

func (s *BlockCommandSuite) processErrorTest(c *gc.C, tstError error, blockType block.Block, expectedError error, expectedWarning string) {
	if tstError != nil {
		c.Assert(errors.Cause(block.ProcessBlockedError(tstError, blockType)), gc.Equals, expectedError)
	} else {
		c.Assert(block.ProcessBlockedError(tstError, blockType), jc.ErrorIsNil)
	}
	// warning displayed
	logOutputText := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Assert(logOutputText, gc.Matches, expectedWarning)
}

func (s *BlockCommandSuite) TestProcessErrOperationBlocked(c *gc.C) {
	s.processErrorTest(c, common.ErrOperationBlocked("operations that remove"), block.BlockRemove, cmd.ErrSilent, ".*operations that remove.*")
	s.processErrorTest(c, common.ErrOperationBlocked("destroy-environment operation has been blocked"), block.BlockDestroy, cmd.ErrSilent, ".*destroy-environment operation has been blocked.*")
}

func (s *BlockCommandSuite) TestProcessErrNil(c *gc.C) {
	s.processErrorTest(c, nil, block.BlockDestroy, nil, "")
}

func (s *BlockCommandSuite) TestProcessErrAny(c *gc.C) {
	err := errors.New("Test error Processing")
	s.processErrorTest(c, err, block.BlockDestroy, err, "")
}
