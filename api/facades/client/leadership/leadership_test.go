// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/facades/client/leadership"
	"github.com/juju/juju/apiserver/params"
	coreleadership "github.com/juju/juju/core/leadership"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	clientFacade *mocks.MockClientFacade
	facade       *mocks.MockFacadeCaller

	client coreleadership.Pinner
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) TestPinLeadershipSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.PinLeadershipBulkParams{Params: []params.PinLeadershipParams{{ApplicationTag: "application-redis"}}}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	s.facade.EXPECT().FacadeCall("PinLeadership", args, gomock.Any()).SetArg(2, resultSource)

	c.Assert(s.client.PinLeadership("redis"), jc.ErrorIsNil)
}

func (s *LeadershipSuite) TestPinLeadershipError(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.PinLeadershipBulkParams{Params: []params.PinLeadershipParams{{ApplicationTag: "application-redis"}}}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{
		{Error: &params.Error{Message: "boom"}},
	}}
	s.facade.EXPECT().FacadeCall("PinLeadership", args, gomock.Any()).SetArg(2, resultSource)

	c.Assert(s.client.PinLeadership("redis"), gc.ErrorMatches, "boom")
}

func (s *LeadershipSuite) TestPinLeadershipMultiReturnError(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.PinLeadershipBulkParams{Params: []params.PinLeadershipParams{{ApplicationTag: "application-redis"}}}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}, {}}}
	s.facade.EXPECT().FacadeCall("PinLeadership", args, gomock.Any()).SetArg(2, resultSource)

	c.Assert(s.client.PinLeadership("redis"), gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *LeadershipSuite) TestUnpinLeadershipSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.PinLeadershipBulkParams{Params: []params.PinLeadershipParams{{ApplicationTag: "application-redis"}}}
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{}}}
	s.facade.EXPECT().FacadeCall("UnpinLeadership", args, gomock.Any()).SetArg(2, resultSource)

	c.Assert(s.client.UnpinLeadership("redis"), jc.ErrorIsNil)
}

func (s *LeadershipSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clientFacade = mocks.NewMockClientFacade(ctrl)
	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.client = leadership.NewClientFromFacades(s.clientFacade, s.facade)

	return ctrl
}
