// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/client/leadership"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership/mocks"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	pinner   *mocks.MockPinner
	modelTag names.ModelTag
	api      leadership.API
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) TestPinSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().PinLeadership("redis", s.modelTag).Return(nil)

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
			EntityTag:      s.modelTag.String(),
		}},
	}
	res, err := s.api.PinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Check(res, gc.DeepEquals, expected)
}

func (s *LeadershipSuite) TestPinError(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().PinLeadership("redis", s.modelTag).Return(errors.New("boom"))

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
			EntityTag:      s.modelTag.String(),
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

	s.pinner.EXPECT().UnpinLeadership("redis", s.modelTag).Return(nil)

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
			EntityTag:      s.modelTag.String(),
		}},
	}
	res, err := s.api.UnpinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Check(res, gc.DeepEquals, expected)
}

func (s *LeadershipSuite) TestUnpinError(c *gc.C) {
	defer s.setup(c).Finish()

	s.pinner.EXPECT().UnpinLeadership("redis", s.modelTag).Return(errors.New("boom"))

	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: "application-redis",
			EntityTag:      s.modelTag.String(),
		}},
	}
	res, err := s.api.UnpinLeadership(arg)
	c.Assert(err, jc.ErrorIsNil)

	results := res.Results
	c.Assert(results, gc.HasLen, 1)
	c.Check(results[0].Error, gc.ErrorMatches, "boom")
}

func (s *LeadershipSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pinner = mocks.NewMockPinner(ctrl)
	s.modelTag = names.NewModelTag(utils.MustNewUUID().String())

	var err error
	s.api, err = leadership.NewAPI(
		s.modelTag,
		s.pinner,
		&apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")},
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
