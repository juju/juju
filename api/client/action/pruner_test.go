// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/rpc/params"
)

type prunerSuite struct{}

var _ = gc.Suite(&prunerSuite{})

func (s *prunerSuite) TestPrune(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.ActionPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("Prune", args, nil).Return(nil)

	client := action.NewPrunerFromCaller(mockFacadeCaller)
	err := client.Prune(time.Hour, 666)
	c.Assert(err, jc.ErrorIsNil)
}
