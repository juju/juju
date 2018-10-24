// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership/mocks"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	backend *commonmocks.MockLeadershipPinningBackend
	machine *commonmocks.MockLeadershipMachine
	pinner  *mocks.MockPinner

	tag         names.Tag
	api         common.LeadershipPinningAPI
	machineApps []string
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.machineApps = []string{"mysql", "redis", "wordpress"}
}

func (s *LeadershipSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.tag = nil
}

func (s *LeadershipSuite) TestPinMachineApplicationsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().PinLeadership(app, s.tag).Return(nil)
	}

	res, err := s.api.PinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestPinMachineApplicationsPartialError(c *gc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().PinLeadership("mysql", s.tag).Return(nil)
	s.pinner.EXPECT().PinLeadership("redis", s.tag).Return(nil)
	s.pinner.EXPECT().PinLeadership("wordpress", s.tag).Return(errorRes)

	res, err := s.api.PinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[2].Error = common.ServerError(errorRes)
	c.Check(res, gc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().UnpinLeadership(app, s.tag).Return(nil)
	}

	res, err := s.api.UnpinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsPartialError(c *gc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().UnpinLeadership("mysql", s.tag).Return(nil)
	s.pinner.EXPECT().UnpinLeadership("redis", s.tag).Return(errorRes)
	s.pinner.EXPECT().UnpinLeadership("wordpress", s.tag).Return(nil)

	res, err := s.api.UnpinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[1].Error = common.ServerError(errorRes)
	c.Check(res, gc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) TestPermissionDenied(c *gc.C) {
	s.tag = names.NewUserTag("some-random-cat")
	defer s.setup(c).Finish()

	_, err := s.api.PinMachineApplications()
	c.Assert(err, gc.ErrorMatches, "permission denied")

	_, err = s.api.UnpinMachineApplications()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *LeadershipSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = commonmocks.NewMockLeadershipPinningBackend(ctrl)
	s.machine = commonmocks.NewMockLeadershipMachine(ctrl)
	s.pinner = mocks.NewMockPinner(ctrl)

	s.backend.EXPECT().Machine("0").Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().ApplicationNames().Return(s.machineApps, nil).AnyTimes()

	if s.tag == nil {
		s.tag = names.NewMachineTag("0")
	}

	var err error
	s.api, err = common.NewLeadershipPinningAPI(
		s.backend,
		names.NewModelTag(utils.MustNewUUID().String()),
		s.pinner,
		&apiservertesting.FakeAuthorizer{Tag: s.tag},
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *LeadershipSuite) pinApplicationsSuccessResults() []params.PinApplicationResult {
	results := make([]params.PinApplicationResult, len(s.machineApps))
	for i, app := range s.machineApps {
		results[i] = params.PinApplicationResult{ApplicationTag: names.NewApplicationTag(app).String()}
	}
	return results
}
