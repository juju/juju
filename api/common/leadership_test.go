// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	apiservercommon "github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	facade *mocks.MockFacadeCaller
	client *common.LeadershipPinningAPI

	machineApps []string
}

var _ = gc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.machineApps = []string{"mysql", "redis", "wordpress"}
}

func (s *LeadershipSuite) TestPinnedLeadership(c *gc.C) {
	defer s.setup(c).Finish()

	pinned := map[string][]string{"redis": {"machine-0", "machine-1"}}
	resultSource := params.PinnedLeadershipResult{Result: pinned}
	s.facade.EXPECT().FacadeCall("PinnedLeadership", nil, gomock.Any()).SetArg(2, resultSource)

	res, err := s.client.PinnedLeadership()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, map[string][]names.Tag{"redis": {names.NewMachineTag("0"), names.NewMachineTag("1")}})
}

func (s *LeadershipSuite) TestPinMachineApplicationsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	resultSource := params.PinApplicationsResults{Results: s.pinApplicationsServerSuccessResults()}
	s.facade.EXPECT().FacadeCall("PinMachineApplications", nil, gomock.Any()).SetArg(2, resultSource)

	res, err := s.client.PinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, s.pinApplicationsClientSuccessResults())
}

func (s *LeadershipSuite) TestPinMachineApplicationsPartialError(c *gc.C) {
	defer s.setup(c).Finish()

	errorRes := apiservercommon.ServerError(errors.New("boom"))
	results := s.pinApplicationsServerSuccessResults()
	results[2].Error = errorRes
	resultSource := params.PinApplicationsResults{Results: results}
	s.facade.EXPECT().FacadeCall("PinMachineApplications", nil, gomock.Any()).SetArg(2, resultSource)

	res, err := s.client.PinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)

	exp := s.pinApplicationsClientSuccessResults()
	exp["wordpress"] = errorRes
	c.Check(res, gc.DeepEquals, exp)
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	resultSource := params.PinApplicationsResults{Results: s.pinApplicationsServerSuccessResults()}
	s.facade.EXPECT().FacadeCall("UnpinMachineApplications", nil, gomock.Any()).SetArg(2, resultSource)

	res, err := s.client.UnpinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, s.pinApplicationsClientSuccessResults())
}

func (s *LeadershipSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.client = common.NewLeadershipPinningAPIFromFacade(s.facade)

	return ctrl
}

func (s *LeadershipSuite) TestUnpinMachineApplicationsPartialError(c *gc.C) {
	defer s.setup(c).Finish()

	errorRes := apiservercommon.ServerError(errors.New("boom"))
	results := s.pinApplicationsServerSuccessResults()
	results[1].Error = errorRes
	resultSource := params.PinApplicationsResults{Results: results}
	s.facade.EXPECT().FacadeCall("UnpinMachineApplications", nil, gomock.Any()).SetArg(2, resultSource)

	res, err := s.client.UnpinMachineApplications()
	c.Assert(err, jc.ErrorIsNil)

	exp := s.pinApplicationsClientSuccessResults()
	exp["redis"] = errorRes
	c.Check(res, gc.DeepEquals, exp)
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
