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
	s.PatchValue(block.BlockClient, func(p *block.BlockCommand) (block.BlockClientAPI, error) {
		return s.mockClient, nil
	})
}

var _ = gc.Suite(&BlockCommandSuite{})

func runBlockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&block.BlockCommand{}), args...)
	return err
}

func (s *BlockCommandSuite) assertRunBlock(c *gc.C, operation, message string) {
	err := runBlockCommand(c, operation, message)
	c.Assert(err, jc.ErrorIsNil)

	expectedOp := block.TranslateOperation(operation)
	c.Assert(s.mockClient.BlockType, gc.DeepEquals, expectedOp)
	c.Assert(s.mockClient.Msg, gc.DeepEquals, message)
}

func (s *BlockCommandSuite) TestBlockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c), `.*must specify one of.*`)
}

func (s *BlockCommandSuite) TestBlockCmdMoreArgs(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment", "change", "too much"), `.*can only specify block type and its message.*`)
}

func (s *BlockCommandSuite) TestBlockCmdOperationWithSeparator(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment|"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdUnknownJujuOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "add-machine"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdUnknownOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "blah"), `.*valid argument.*`)
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperationUpperCase(c *gc.C) {
	s.assertRunBlock(c, "DESTROY-ENVIRONMENT", "TestBlockCmdValidDestroyEnvOperationUpperCase")
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperation(c *gc.C) {
	s.assertRunBlock(c, "destroy-environment", "TestBlockCmdValidDestroyEnvOperation")
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
