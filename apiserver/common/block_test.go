// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type blockCheckerSuite struct {
	aBlock                  state.Block
	destroy, remove, change state.Block

	service      *mocks.MockBlockCommandService
	blockchecker *common.BlockChecker
}

var _ = gc.Suite(&blockCheckerSuite{})

func (s *blockCheckerSuite) TestDestroyBlockChecker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return(s.destroy.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), s.destroy.Message())

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return(s.remove.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), s.remove.Message())

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.DestroyBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return(s.change.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.DestroyAllowed(context.Background()), s.change.Message())
}

func (s *blockCheckerSuite) TestRemoveBlockChecker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return(s.remove.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(context.Background()), s.remove.Message())

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.RemoveBlock).Return("", blockcommanderrors.NotFound)
	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return(s.change.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.RemoveAllowed(context.Background()), s.change.Message())
}

func (s *blockCheckerSuite) TestChangeBlockChecker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.service.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return(s.change.Message(), nil)
	s.assertErrorBlocked(c, true, s.blockchecker.ChangeAllowed(context.Background()), s.change.Message())
}

func (s *blockCheckerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = mocks.NewMockBlockCommandService(ctrl)
	s.blockchecker = common.NewBlockChecker(s.service)
	return ctrl
}

func (s *blockCheckerSuite) assertErrorBlocked(c *gc.C, blocked bool, err error, msg string) {
	if blocked {
		c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue)
		c.Assert(err, gc.ErrorMatches, msg)
	} else {
		c.Assert(errors.Cause(err), jc.ErrorIsNil)
	}
}
