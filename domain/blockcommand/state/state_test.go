// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *stdtesting.T) { tc.Run(t, &stateSuite{}) }
func (s *stateSuite) TestSetBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "block-message")

	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestSetBlockForSameTypeTwice(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, tc.ErrorIsNil)
	err = st.SetBlock(c.Context(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, tc.ErrorIs, blockcommanderrors.AlreadyExists)
}

func (s *stateSuite) TestSetBlockWithNoMessage(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestRemoveBlockWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.RemoveBlock(c.Context(), blockcommand.DestroyBlock)

	c.Assert(err, tc.ErrorIs, blockcommanderrors.NotFound)
}

func (s *stateSuite) TestRemoveBlockWithExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "")
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveBlock(c.Context(), blockcommand.DestroyBlock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestGetBlocksWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	blocks, err := st.GetBlocks(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 0)
}

func (s *stateSuite) TestGetBlocks(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "")
	c.Assert(err, tc.ErrorIsNil)
	err = st.SetBlock(c.Context(), blockcommand.ChangeBlock, "change me")
	c.Assert(err, tc.ErrorIsNil)

	blocks, err := st.GetBlocks(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 2)
	c.Check(blocks[0].Type, tc.Equals, blockcommand.DestroyBlock)
	c.Check(blocks[0].Message, tc.Equals, "")
	c.Check(blocks[1].Type, tc.Equals, blockcommand.ChangeBlock)
	c.Check(blocks[1].Message, tc.Equals, "change me")
}

func (s *stateSuite) TestGetBlockMessageWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	message, err := st.GetBlockMessage(c.Context(), blockcommand.DestroyBlock)

	c.Assert(err, tc.ErrorIs, blockcommanderrors.NotFound)
	c.Assert(message, tc.Equals, "")
}

func (s *stateSuite) TestGetBlockMessage(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "destroy me")
	c.Assert(err, tc.ErrorIsNil)

	message, err := st.GetBlockMessage(c.Context(), blockcommand.DestroyBlock)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(message, tc.Equals, "destroy me")
}

func (s *stateSuite) TestRemoveAllBlocksWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.RemoveAllBlocks(c.Context())

	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestRemoveAllBlocksWithExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(c.Context(), blockcommand.DestroyBlock, "")
	c.Assert(err, tc.ErrorIsNil)

	err = st.RemoveAllBlocks(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}
