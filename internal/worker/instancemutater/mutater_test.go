// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiinstancemutater "github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/instancemutater"
	"github.com/juju/juju/internal/worker/instancemutater/mocks"
)

type mutaterSuite struct {
	testhelpers.IsolationSuite

	tag    names.MachineTag
	instId string

	facade  *mocks.MockInstanceMutaterAPI
	machine *mocks.MockMutaterMachine
	broker  *mocks.MockLXDProfiler

	mutaterMachine *instancemutater.MutaterMachine
}

var _ = tc.Suite(&mutaterSuite{})

func (s *mutaterSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("2")
	s.instId = "juju-23413-2"
	s.IsolationSuite.SetUpTest(c)
}

func (s *mutaterSuite) TestProcessMachineProfileChanges(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}
	finishingProfiles := append(startingProfiles, "juju-testme-lxd-profile-1")
	charmProfiles := []string{"juju-testme-lxd-profile-1"}

	s.expectRefreshLifeAliveStatusIdle()
	s.expectLXDProfileNames(startingProfiles, nil)
	s.expectAssignLXDProfiles(finishingProfiles, nil)
	s.expectSetCharmProfiles(charmProfiles)
	s.expectModificationStatusApplied()

	info := s.info(startingProfiles, 1, true)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mutaterSuite) TestProcessMachineProfileChangesMachineDead(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}

	s.expectRefreshLifeDead()

	info := s.info(startingProfiles, 1, false)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *mutaterSuite) TestProcessMachineProfileChangesError(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}
	finishingProfiles := append(startingProfiles, "juju-testme-lxd-profile-1")

	s.expectRefreshLifeAliveStatusIdle()
	s.expectLXDProfileNames(startingProfiles, nil)
	s.expectAssignLXDProfiles(finishingProfiles, errors.New("fail me"))
	s.expectModificationStatusError()

	info := s.info(startingProfiles, 1, true)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, tc.ErrorMatches, "fail me")
}

func (s *mutaterSuite) TestProcessMachineProfileChangesNilInfo(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, &apiinstancemutater.UnitProfileInfo{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *mutaterSuite) TestGatherProfileDataReplace(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 1, true),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(post, tc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-0", Profile: nil},
		{Name: "juju-testme-lxd-profile-1", Profile: &testProfile},
	})
}

func (s *mutaterSuite) TestGatherProfileDataRemove(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 0, false),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(post, tc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-0", Profile: nil},
	})
}

func (s *mutaterSuite) TestGatherProfileDataAdd(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme"}, 1, true),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(post, tc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-1", Profile: &testProfile},
	})
}

func (s *mutaterSuite) TestGatherProfileDataNoChange(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 0, true),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(post, tc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-0", Profile: &testProfile},
	})
}

func (s *mutaterSuite) info(profiles []string, rev int, add bool) *apiinstancemutater.UnitProfileInfo {
	info := &apiinstancemutater.UnitProfileInfo{
		ModelName:       "testme",
		InstanceId:      instance.Id(s.instId),
		CurrentProfiles: profiles,
		ProfileChanges: []apiinstancemutater.UnitProfileChanges{
			{ApplicationName: "lxd-profile",
				Revision: rev,
			},
			// app-no-profile tests the fix for lp:1904619,
			// use a pointer to a copy of data in for loop,
			// rather than a pointer to the data as the data
			// will change in subsequent loop iterations, but
			// the pointer's address will not.
			{ApplicationName: "app-no-profile",
				Revision: rev,
			},
		},
	}
	if add {
		info.ProfileChanges[0].Profile = testProfile
	}
	return info
}

func (s *mutaterSuite) TestVerifyCurrentProfilesTrue(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, profiles)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
	c.Assert(obtained, tc.DeepEquals, profiles)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseLength(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, append(profiles, "juju-testme-next-1"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
	c.Assert(obtained, tc.DeepEquals, profiles)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseContents(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default", "juju-testme", "juju-testme-lxd-profile-1"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
	c.Assert(obtained, tc.DeepEquals, profiles)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseContentsWithMissingExpectedProfiles(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default", "juju-testme"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
	c.Assert(obtained, tc.DeepEquals, profiles)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseContentsWithMissingProviderProfiles(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme"}
	s.expectLXDProfileNames(profiles, nil)

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default", "juju-testme", "juju-testme-lxd-profile-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
	c.Assert(obtained, tc.DeepEquals, profiles)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesError(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.expectLXDProfileNames([]string{}, errors.NotFoundf("instId"))

	ok, obtained, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default"})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(ok, tc.IsFalse)
	c.Assert(obtained, tc.IsNil)
}

func (s *mutaterSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	logger := loggertesting.WrapCheckLog(c)

	s.machine = mocks.NewMockMutaterMachine(ctrl)
	s.machine.EXPECT().Tag().Return(s.tag).AnyTimes()

	s.broker = mocks.NewMockLXDProfiler(ctrl)
	s.facade = mocks.NewMockInstanceMutaterAPI(ctrl)

	s.mutaterMachine = instancemutater.NewMachineContext(logger, s.broker, s.machine, s.getRequiredLXDProfiles, s.tag.Id())
	return ctrl
}

func (s *mutaterSuite) expectLXDProfileNames(profiles []string, err error) {
	s.broker.EXPECT().LXDProfileNames(s.instId).Return(profiles, err)
}

func (s *mutaterSuite) expectRefreshLifeAliveStatusIdle() {
	mExp := s.machine.EXPECT()
	mExp.Refresh(gomock.Any()).Return(nil)
	mExp.Life().Return(life.Alive)
	mExp.SetModificationStatus(gomock.Any(), status.Idle, "", nil).Return(nil)
}

func (s *mutaterSuite) expectRefreshLifeDead() {
	mExp := s.machine.EXPECT()
	mExp.Refresh(gomock.Any()).Return(nil)
	mExp.Life().Return(life.Dead)
}

func (s *mutaterSuite) expectModificationStatusApplied() {
	s.machine.EXPECT().SetModificationStatus(gomock.Any(), status.Applied, "", nil).Return(nil)
}

func (s *mutaterSuite) expectModificationStatusError() {
	s.machine.EXPECT().SetModificationStatus(gomock.Any(), status.Error, gomock.Any(), gomock.Any()).Return(nil)
}

func (s *mutaterSuite) expectAssignLXDProfiles(profiles []string, err error) {
	s.broker.EXPECT().AssignLXDProfiles(s.instId, profiles, gomock.Any()).Return(profiles, err)
}

func (s *mutaterSuite) expectSetCharmProfiles(profiles []string) {
	s.machine.EXPECT().SetCharmProfiles(gomock.Any(), profiles)
}

func (s *mutaterSuite) getRequiredLXDProfiles(modelName string) []string {
	return []string{"default", "juju-" + modelName}
}

var testProfile = lxdprofile.Profile{
	Config: map[string]string{
		"security.nesting": "true",
	},
	Description: "dummy profile description",
	Devices: map[string]map[string]string{
		"tun": {
			"path": "/dev/net/tun",
		},
	},
}
