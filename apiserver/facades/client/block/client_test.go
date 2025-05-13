// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/rpc/params"
)

type blockSuite struct {
	api *API

	service    *MockBlockCommandService
	authorizer *MockAuthorizer
}

var _ = tc.Suite(&blockSuite{})

func (s *blockSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockBlockCommandService(ctrl)
	s.authorizer = NewMockAuthorizer(ctrl)

	s.api = &API{
		modelTag:   names.NewModelTag("beef1beef1-0000-0000-000011112222"),
		service:    s.service,
		authorizer: s.authorizer,
	}

	return ctrl
}

func (s *blockSuite) TestListBlockNoneExistent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, s.api.modelTag).Return(nil)
	s.service.EXPECT().GetBlocks(gomock.Any()).Return(nil, nil)

	s.assertBlockList(c, 0)
}

func (s *blockSuite) TestSwitchValidBlockOn(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, s.api.modelTag).Return(nil)
	s.service.EXPECT().SwitchBlockOn(gomock.Any(), blockcommand.DestroyBlock, "for TestSwitchValidBlockOn").Return(nil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, s.api.modelTag).Return(nil)
	s.service.EXPECT().GetBlocks(gomock.Any()).Return([]blockcommand.Block{
		{Type: blockcommand.DestroyBlock, Message: "for TestSwitchValidBlockOn"},
	}, nil)

	s.assertSwitchBlockOn(c, params.BlockDestroy, "for TestSwitchValidBlockOn")
}

func (s *blockSuite) TestSwitchInvalidBlockOn(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, s.api.modelTag).Return(nil)

	on := params.BlockSwitchParams{
		Type:    "invalid_block_type",
		Message: "for TestSwitchInvalidBlockOn",
	}

	err := s.api.SwitchBlockOn(context.Background(), on)
	c.Assert(err.Error, tc.NotNil)
}

func (s *blockSuite) TestSwitchBlockOff(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, s.api.modelTag).Return(nil)
	s.service.EXPECT().SwitchBlockOff(gomock.Any(), blockcommand.DestroyBlock).Return(nil)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, s.api.modelTag).Return(nil)
	s.service.EXPECT().GetBlocks(gomock.Any()).Return(nil, nil)

	off := params.BlockSwitchParams{
		Type: params.BlockDestroy,
	}
	err := s.api.SwitchBlockOff(context.Background(), off)
	c.Assert(err.Error, tc.IsNil)
	s.assertBlockList(c, 0)
}

func (s *blockSuite) assertBlockList(c *tc.C, length int) {
	all, err := s.api.List(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(all.Results, tc.HasLen, length)
}

func (s *blockSuite) assertSwitchBlockOn(c *tc.C, blockType, msg string) {
	on := params.BlockSwitchParams{
		Type:    blockType,
		Message: msg,
	}
	err := s.api.SwitchBlockOn(context.Background(), on)
	c.Assert(err.Error, tc.IsNil)
	s.assertBlockList(c, 1)
}
