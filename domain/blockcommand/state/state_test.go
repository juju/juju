// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) TestSetBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")

	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSetBlockForSameTypeTwice(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, jc.ErrorIs, blockcommanderrors.AlreadyExists)
}

func (s *stateSuite) TestSetBlockWithNoMessage(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRemoveBlockWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.RemoveBlock(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIs, blockcommanderrors.NotFound)
}

func (s *stateSuite) TestRemoveBlockWithExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveBlock(context.Background(), blockcommand.DestroyBlock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetBlocksWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	blocks, err := st.GetBlocks(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 0)
}

func (s *stateSuite) TestGetBlocks(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetBlock(context.Background(), blockcommand.ChangeBlock, "change me")
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := st.GetBlocks(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 2)
	c.Check(blocks[0].Type, tc.Equals, blockcommand.DestroyBlock)
	c.Check(blocks[0].Message, tc.Equals, "")
	c.Check(blocks[1].Type, tc.Equals, blockcommand.ChangeBlock)
	c.Check(blocks[1].Message, tc.Equals, "change me")
}

func (s *stateSuite) TestGetBlockMessageWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	message, err := st.GetBlockMessage(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIs, blockcommanderrors.NotFound)
	c.Assert(message, tc.Equals, "")
}

func (s *stateSuite) TestGetBlockMessage(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "destroy me")
	c.Assert(err, jc.ErrorIsNil)

	message, err := st.GetBlockMessage(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(message, tc.Equals, "destroy me")
}

func (s *stateSuite) TestRemoveAllBlocksWithNoExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.RemoveAllBlocks(context.Background())

	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRemoveAllBlocksWithExistingBlock(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveAllBlocks(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}
