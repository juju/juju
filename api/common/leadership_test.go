// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	facade *mocks.MockFacadeCaller
	client *common.LeadershipPinningAPI

	machineApps []string
}

var _ = tc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.machineApps = []string{"mysql", "redis", "wordpress"}
}

func (s *LeadershipSuite) TestPinnedLeadership(c *tc.C) {
	defer s.setup(c).Finish()

	pinned := map[string][]string{"redis": {"machine-0", "machine-1"}}
	resultSource := params.PinnedLeadershipResult{Result: pinned}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "PinnedLeadership", nil, gomock.Any()).SetArg(3, resultSource)

	res, err := s.client.PinnedLeadership(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, map[string][]names.Tag{"redis": {names.NewMachineTag("0"), names.NewMachineTag("1")}})
}

func (s *LeadershipSuite) TestPinnedLeadershipError(c *tc.C) {
	defer s.setup(c).Finish()

	resultSource := params.PinnedLeadershipResult{Error: apiservererrors.ServerError(errors.New("splat"))}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "PinnedLeadership", nil, gomock.Any()).SetArg(3, resultSource)

	_, err := s.client.PinnedLeadership(context.Background())
	c.Assert(err, tc.ErrorMatches, "splat")
}

func (s *LeadershipSuite) TestPinMachineApplicationsSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	resultSource := params.PinApplicationsResults{Results: s.pinApplicationsServerSuccessResults()}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "PinMachineApplications", nil, gomock.Any()).SetArg(3, resultSource)

	res, err := s.client.PinMachineApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, s.pinApplicationsClientSuccessResults())
}

func (s *LeadershipSuite) TestPinMachineApplicationsPartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := apiservererrors.ServerError(errors.New("boom"))
	results := s.pinApplicationsServerSuccessResults()
	results[2].Error = errorRes
	resultSource := params.PinApplicationsResults{Results: results}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "PinMachineApplications", nil, gomock.Any()).SetArg(3, resultSource)

	res, err := s.client.PinMachineApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	exp := s.pinApplicationsClientSuccessResults()
	exp["wordpress"] = errorRes
	c.Check(res, tc.DeepEquals, exp)
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	resultSource := params.PinApplicationsResults{Results: s.pinApplicationsServerSuccessResults()}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "UnpinMachineApplications", nil, gomock.Any()).SetArg(3, resultSource)

	res, err := s.client.UnpinMachineApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, s.pinApplicationsClientSuccessResults())
}

func (s *LeadershipSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.client = common.NewLeadershipPinningAPIFromFacade(s.facade)

	return ctrl
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsPartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := apiservererrors.ServerError(errors.New("boom"))
	results := s.pinApplicationsServerSuccessResults()
	results[1].Error = errorRes
	resultSource := params.PinApplicationsResults{Results: results}
	s.facade.EXPECT().FacadeCall(gomock.Any(), "UnpinMachineApplications", nil, gomock.Any()).SetArg(3, resultSource)

	res, err := s.client.UnpinMachineApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	exp := s.pinApplicationsClientSuccessResults()
	exp["redis"] = errorRes
	c.Check(res, tc.DeepEquals, exp)
}

func (s *LeadershipSuite) pinApplicationsServerSuccessResults() []params.PinApplicationResult {
	results := make([]params.PinApplicationResult, len(s.machineApps))
	for i, app := range s.machineApps {
		results[i] = params.PinApplicationResult{ApplicationName: app}
	}
	return results
}

func (s *LeadershipSuite) pinApplicationsClientSuccessResults() map[string]error {
	results := make(map[string]error, len(s.machineApps))
	for _, app := range s.machineApps {
		results[app] = nil
	}
	return results
}
