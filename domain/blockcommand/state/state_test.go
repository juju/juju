// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestSetBlock(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")

	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSetBlockForSameTypeTwice(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetBlock(context.Background(), blockcommand.DestroyBlock, "block-message")
	c.Assert(err, jc.ErrorIs, blockcommanderrors.AlreadyExists)
}

func (s *stateSuite) TestSetBlockWithNoMessage(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestRemoveBlockWithNoExistingBlock(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.RemoveBlock(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIs, blockcommanderrors.NotFound)
}

func (s *stateSuite) TestRemoveBlockWithExistingBlock(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)

	err = st.RemoveBlock(context.Background(), blockcommand.DestroyBlock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetBlocksWithNoExistingBlock(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	blocks, err := st.GetBlocks(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 0)
}

func (s *stateSuite) TestGetBlocks(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetBlock(context.Background(), blockcommand.ChangeBlock, "change me")
	c.Assert(err, jc.ErrorIsNil)

	blocks, err := st.GetBlocks(context.Background())

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 2)
	c.Check(blocks[0].Type, gc.Equals, blockcommand.DestroyBlock)
	c.Check(blocks[0].Message, gc.Equals, "")
	c.Check(blocks[1].Type, gc.Equals, blockcommand.ChangeBlock)
	c.Check(blocks[1].Message, gc.Equals, "change me")
}

func (s *stateSuite) TestGetBlockMessageWithNoExistingBlock(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	message, err := st.GetBlockMessage(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIs, blockcommanderrors.NotFound)
	c.Assert(message, gc.Equals, "")
}

func (s *stateSuite) TestGetBlockMessage(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	err := st.SetBlock(context.Background(), blockcommand.DestroyBlock, "destroy me")
	c.Assert(err, jc.ErrorIsNil)

	message, err := st.GetBlockMessage(context.Background(), blockcommand.DestroyBlock)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(message, gc.Equals, "destroy me")
}
