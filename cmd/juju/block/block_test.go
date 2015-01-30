// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type BlockCommandSuite struct {
	ProtectionCommandSuite
}

var _ = gc.Suite(&BlockCommandSuite{})

func runBlockCommand(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&block.BlockCommand{}), args...)
	return err
}

func (s *BlockCommandSuite) runBlockTestAndCompare(c *gc.C, operation string, expectedValue bool) {
	err := runBlockCommand(c, operation)
	c.Assert(err, jc.ErrorIsNil)

	expectedOp := config.BlockKeyPrefix + strings.ToLower(operation)
	expectedCfg := map[string]interface{}{expectedOp: expectedValue}
	c.Assert(s.mockClient.cfg, gc.DeepEquals, expectedCfg)
}

func (s *BlockCommandSuite) TestBlockCmdNoOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c), `.*must specify one of.*`)
}

func (s *BlockCommandSuite) TestBlockCmdMoreThanOneOperation(c *gc.C) {
	s.assertErrorMatches(c, runBlockCommand(c, "destroy-environment", "change"), `.*must specify one of.*`)
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
	s.runBlockTestAndCompare(c, "DESTROY-ENVIRONMENT", true)
}

func (s *BlockCommandSuite) TestBlockCmdValidDestroyEnvOperation(c *gc.C) {
	s.runBlockTestAndCompare(c, "destroy-environment", true)
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
	s.processErrorTest(c, common.ErrOperationBlocked, block.BlockRemove, cmd.ErrSilent, ".*operations that remove.*")
	s.processErrorTest(c, common.ErrOperationBlocked, block.BlockDestroy, cmd.ErrSilent, ".*destroy-environment operation has been blocked.*")
}

func (s *BlockCommandSuite) TestProcessErrNil(c *gc.C) {
	s.processErrorTest(c, nil, block.BlockDestroy, nil, "")
}

func (s *BlockCommandSuite) TestProcessErrAny(c *gc.C) {
	err := errors.New("Test error Processing")
	s.processErrorTest(c, err, block.BlockDestroy, err, "")
}
