// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSwitchOnBlock(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().SetBlock(gomock.Any(), blockcommand.RemoveBlock, "block-message").Return(nil)

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, "block-message")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSwitchOnBlockWithTooLargeMessage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, strings.Repeat("a", blockcommand.DefaultMaxMessageLength+1))
	c.Assert(err, tc.ErrorMatches, `message length exceeds maximum allowed length of \d+`)
}

func (s *serviceSuite) TestSwitchOnBlockAlreadyExists(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().SetBlock(gomock.Any(), blockcommand.RemoveBlock, "block-message").Return(blockcommanderrors.AlreadyExists)

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, "block-message")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSwitchOffBlock(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().RemoveBlock(gomock.Any(), blockcommand.RemoveBlock).Return(nil)

	err := s.service(c).SwitchBlockOff(context.Background(), blockcommand.RemoveBlock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetBlocks(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{
		{Type: blockcommand.RemoveBlock, Message: "block-message"},
	}, nil)

	blocks, err := s.service(c).GetBlocks(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocks, tc.HasLen, 1)
	c.Check(blocks, tc.DeepEquals, []blockcommand.Block{
		{Type: blockcommand.RemoveBlock, Message: "block-message"},
	})
}

func (s *serviceSuite) TestGetBlockMessage(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().GetBlockMessage(gomock.Any(), blockcommand.RemoveBlock).Return("foo", nil)

	message, err := s.service(c).GetBlockSwitchedOn(context.Background(), blockcommand.RemoveBlock)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(message, tc.Equals, "foo")
}

func (s *serviceSuite) TestRemoveAllBlocks(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().RemoveAllBlocks(gomock.Any()).Return(nil)

	err := s.service(c).RemoveAllBlocks(context.Background())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) service(c *tc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c))
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
