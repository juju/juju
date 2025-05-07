// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership/mocks"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type LeadershipSuite struct {
	coretesting.BaseSuite

	backend *commonmocks.MockLeadershipPinningBackend
	machine *commonmocks.MockLeadershipMachine
	pinner  *mocks.MockPinner

	modelTag    names.ModelTag
	authTag     names.Tag
	api         *common.LeadershipPinning
	machineApps []string
}

var _ = tc.Suite(&LeadershipSuite{})

func (s *LeadershipSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)

	s.modelTag = names.NewModelTag(uuid.MustNewUUID().String())
	s.machineApps = []string{"mysql", "redis", "wordpress"}
}

func (s *LeadershipSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.authTag = nil
}

func (s *LeadershipSuite) TestPinnedLeadershipSuccess(c *tc.C) {
	s.authTag = names.NewUserTag("admin")
	defer s.setup(c).Finish()

	pinned := map[string][]string{"redis": {"machine-0", "machine-1"}}
	s.pinner.EXPECT().PinnedLeadership().Return(pinned, nil)

	res, err := s.api.PinnedLeadership(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Result, tc.DeepEquals, pinned)
}

func (s *LeadershipSuite) TestPinnedLeadershipPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	_, err := s.api.PinnedLeadership(context.Background())
	c.Check(err, tc.ErrorMatches, "permission denied")
}

func (s *LeadershipSuite) TestPinApplicationLeadersSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().PinLeadership(app, s.authTag.String()).Return(nil)
	}

	res, err := s.api.PinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestPinApplicationLeadersPartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().PinLeadership("mysql", s.authTag.String()).Return(nil)
	s.pinner.EXPECT().PinLeadership("redis", s.authTag.String()).Return(nil)
	s.pinner.EXPECT().PinLeadership("wordpress", s.authTag.String()).Return(errorRes)

	res, err := s.api.PinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[2].Error = apiservererrors.ServerError(errorRes)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) TestUnpinApplicationLeadersSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().UnpinLeadership(app, s.authTag.String()).Return(nil)
	}

	res, err := s.api.UnpinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestUnpinApplicationLeadersPartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().UnpinLeadership("mysql", s.authTag.String()).Return(nil)
	s.pinner.EXPECT().UnpinLeadership("redis", s.authTag.String()).Return(errorRes)
	s.pinner.EXPECT().UnpinLeadership("wordpress", s.authTag.String()).Return(nil)

	res, err := s.api.UnpinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[1].Error = apiservererrors.ServerError(errorRes)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) TestPinApplicationLeadersPermissionDenied(c *tc.C) {
	s.authTag = names.NewUserTag("some-random-cat")
	defer s.setup(c).Finish()

	_, err := s.api.PinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorMatches, "permission denied")

	_, err = s.api.UnpinApplicationLeaders(context.Background())
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *LeadershipSuite) TestGetMachineApplicationNamesSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	appNames, err := s.api.GetMachineApplicationNames(context.Background(), s.authTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appNames, tc.DeepEquals, s.machineApps)
}

func (s *LeadershipSuite) TestPinApplicationLeadersByNameSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().PinLeadership(app, s.authTag.String()).Return(nil)
	}

	res, err := s.api.PinApplicationLeadersByName(context.Background(), s.authTag, s.machineApps)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestPinApplicationLeadersByNamePartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().PinLeadership("mysql", s.authTag.String()).Return(nil)
	s.pinner.EXPECT().PinLeadership("redis", s.authTag.String()).Return(errorRes)
	s.pinner.EXPECT().PinLeadership("wordpress", s.authTag.String()).Return(nil)

	res, err := s.api.PinApplicationLeadersByName(context.Background(), s.authTag, s.machineApps)
	c.Assert(err, tc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[1].Error = apiservererrors.ServerError(errorRes)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) TestUnpinApplicationLeadersByNameSuccess(c *tc.C) {
	defer s.setup(c).Finish()

	for _, app := range s.machineApps {
		s.pinner.EXPECT().UnpinLeadership(app, s.authTag.String()).Return(nil)
	}

	res, err := s.api.UnpinApplicationLeadersByName(context.Background(), s.authTag, s.machineApps)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: s.pinApplicationsSuccessResults()})
}

func (s *LeadershipSuite) TestUnpinApplicationLeadersByNamePartialError(c *tc.C) {
	defer s.setup(c).Finish()

	errorRes := errors.New("boom")
	s.pinner.EXPECT().UnpinLeadership("mysql", s.authTag.String()).Return(nil)
	s.pinner.EXPECT().UnpinLeadership("redis", s.authTag.String()).Return(errorRes)
	s.pinner.EXPECT().UnpinLeadership("wordpress", s.authTag.String()).Return(nil)

	res, err := s.api.UnpinApplicationLeadersByName(context.Background(), s.authTag, s.machineApps)
	c.Assert(err, tc.ErrorIsNil)

	results := s.pinApplicationsSuccessResults()
	results[1].Error = apiservererrors.ServerError(errorRes)
	c.Check(res, tc.DeepEquals, params.PinApplicationsResults{Results: results})
}

func (s *LeadershipSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.backend = commonmocks.NewMockLeadershipPinningBackend(ctrl)
	s.machine = commonmocks.NewMockLeadershipMachine(ctrl)
	s.pinner = mocks.NewMockPinner(ctrl)

	s.backend.EXPECT().Machine("0").Return(s.machine, nil).AnyTimes()
	s.machine.EXPECT().ApplicationNames().Return(s.machineApps, nil).AnyTimes()

	if s.authTag == nil {
		s.authTag = names.NewMachineTag("0")
	}

	var err error
	s.api, err = common.NewLeadershipPinning(
		s.backend,
		s.modelTag,
		s.pinner,
		&apiservertesting.FakeAuthorizer{Tag: s.authTag},
	)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func (s *LeadershipSuite) pinApplicationsSuccessResults() []params.PinApplicationResult {
	results := make([]params.PinApplicationResult, len(s.machineApps))
	for i, app := range s.machineApps {
		results[i] = params.PinApplicationResult{ApplicationName: app}
	}
	return results
}
