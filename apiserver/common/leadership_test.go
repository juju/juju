// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership/mocks"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	pinner *mocks.MockPinner
	tag    names.Tag
	api    common.LeadershipPinningAPI
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.tag = nil
}

func (s *LeadershipSuite) TestPinSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().PinLeadership("redis", s.tag).Return(nil)

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
		}},
	}
	res, err := s.api.PinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Check(res, gc.DeepEquals, expected)
}

func (s *LeadershipSuite) TestPinError(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().PinLeadership("redis", s.tag).Return(errors.New("boom"))

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
		}},
	}
	res, err := s.api.PinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	results := res.Results
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "boom")
}

func (s *LeadershipSuite) TestUnpinSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().UnpinLeadership("redis", s.tag).Return(nil)

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
		}},
	}
	res, err := s.api.UnpinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Check(res, gc.DeepEquals, expected)
}

func (s *LeadershipSuite) TestUnpinError(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().UnpinLeadership("redis", s.tag).Return(errors.New("boom"))

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
		}},
	}
	res, err := s.api.UnpinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	results := res.Results
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "boom")
}

func (s *LeadershipSuite) TestPermissionDenied(c *gc.C) {
	s.tag = names.NewUserTag("some-random-cat")
	defer s.setup(c).Finish()

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
		}},
	}
	_, err := s.api.UnpinLeadership(arg)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *LeadershipSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pinner = mocks.NewMockPinner(ctrl)

	if s.tag == nil {
		s.tag = names.NewMachineTag("0")
	}

	var err error
	s.api, err = common.NewLeadershipPinningAPI(
		names.NewModelTag(utils.MustNewUUID().String()),
		s.pinner,
		&apiservertesting.FakeAuthorizer{Tag: s.tag},
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
