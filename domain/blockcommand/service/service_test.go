// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSwitchOnBlock(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().SetBlock(gomock.Any(), blockcommand.RemoveBlock, "block-message").Return(nil)

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, "block-message")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSwitchOnBlockWithTooLargeMessage(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, strings.Repeat("a", blockcommand.DefaultMaxMessageLength+1))
	c.Assert(err, gc.ErrorMatches, `message length exceeds maximum allowed length of \d+`)
}

func (s *serviceSuite) TestSwitchOnBlockAlreadyExists(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().SetBlock(gomock.Any(), blockcommand.RemoveBlock, "block-message").Return(blockcommanderrors.AlreadyExists)

	err := s.service(c).SwitchBlockOn(context.Background(), blockcommand.RemoveBlock, "block-message")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSwitchOffBlock(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().RemoveBlock(gomock.Any(), blockcommand.RemoveBlock).Return(nil)

	err := s.service(c).SwitchBlockOff(context.Background(), blockcommand.RemoveBlock)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetBlocks(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.state.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{
		{Type: blockcommand.RemoveBlock, Message: "block-message"},
	}, nil)

	blocks, err := s.service(c).GetBlocks(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocks, gc.HasLen, 1)
	c.Check(blocks, jc.DeepEquals, []blockcommand.Block{
		{Type: blockcommand.RemoveBlock, Message: "block-message"},
	})
}

func (s *serviceSuite) service(c *gc.C) *Service {
	return NewService(s.state, loggertesting.WrapCheckLog(c))
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
