// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/rpc/params"
)

type blockCheckerSuite struct {
	service      *mocks.MockBlockCommandService
	blockchecker *common.BlockChecker
}

var _ = tc.Suite(&blockCheckerSuite{})

func (s *blockCheckerSuite) TestDestroyBlockChecker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return("block", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), "block")

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("remove", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), "remove")

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("change", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), "change")
}

func (s *blockCheckerSuite) TestRemoveBlockChecker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("remove", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(context.Background()), "remove")

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("change", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(context.Background()), "change")
}

func (s *blockCheckerSuite) TestChangeBlockChecker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("change", nil)
	s.assertErrorBlocked(c, true, s.blockchecker.ChangeAllowed(context.Background()), "change")
}

func (s *blockCheckerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = mocks.NewMockBlockCommandService(ctrl)
	s.blockchecker = common.NewBlockChecker(s.service)
	return ctrl
}

func (s *blockCheckerSuite) assertErrorBlocked(c *tc.C, blocked bool, err error, msg string) {
	if blocked {
		c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue)
		c.Assert(err, tc.ErrorMatches, msg)
	} else {
		c.Assert(errors.Cause(err), tc.ErrorIsNil)
	}
}
