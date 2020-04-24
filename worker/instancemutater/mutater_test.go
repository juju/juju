// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiinstancemutater "github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
)

type mutaterSuite struct {
	testing.IsolationSuite

	tag    names.MachineTag
	instId string

	facade  *mocks.MockInstanceMutaterAPI
	machine *mocks.MockMutaterMachine
	broker  *mocks.MockLXDProfiler

	mutaterMachine *instancemutater.MutaterMachine
}

var _ = gc.Suite(&mutaterSuite{})

func (s *mutaterSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("2")
	s.instId = "juju-23413-2"
	s.IsolationSuite.SetUpTest(c)
}

func (s *mutaterSuite) TestProcessMachineProfileChanges(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}
	finishingProfiles := append(startingProfiles, "juju-testme-lxd-profile-1")

	s.expectRefreshLifeAliveStatusIdle()
	s.expectLXDProfileNames(startingProfiles, nil)
	s.expectAssignLXDProfiles(finishingProfiles, nil)
	s.expectSetCharmProfiles(finishingProfiles)
	s.expectModificationStatusApplied()

	info := s.info(startingProfiles, 1, true)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mutaterSuite) TestProcessMachineProfileChangesMachineDead(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}

	s.expectRefreshLifeDead()

	info := s.info(startingProfiles, 1, false)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *mutaterSuite) TestProcessMachineProfileChangesError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	startingProfiles := []string{"default", "juju-testme"}
	finishingProfiles := append(startingProfiles, "juju-testme-lxd-profile-1")

	s.expectRefreshLifeAliveStatusIdle()
	s.expectLXDProfileNames(startingProfiles, nil)
	s.expectAssignLXDProfiles(finishingProfiles, errors.New("fail me"))
	s.expectModificationStatusError()

	info := s.info(startingProfiles, 1, true)
	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, info)
	c.Assert(err, gc.ErrorMatches, "fail me")
}

func (s *mutaterSuite) TestProcessMachineProfileChangesNilInfo(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	err := instancemutater.ProcessMachineProfileChanges(s.mutaterMachine, &apiinstancemutater.UnitProfileInfo{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mutaterSuite) TestGatherProfileDataReplace(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 1, true),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(post, gc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-0", Profile: nil},
		{Name: "juju-testme-lxd-profile-1", Profile: &testProfile},
	})
}

func (s *mutaterSuite) TestGatherProfileDataRemove(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 0, false),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(post, gc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-0", Profile: nil},
	})
}

func (s *mutaterSuite) TestGatherProfileDataAdd(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme"}, 1, true),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(post, gc.DeepEquals, []lxdprofile.ProfilePost{
		{Name: "juju-testme-lxd-profile-1", Profile: &testProfile},
	})
}

func (s *mutaterSuite) TestGatherProfileDataNoChange(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	post, err := instancemutater.GatherProfileData(
		s.mutaterMachine,
		s.info([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, 0, true),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(post, gc.DeepEquals, []lxdprofile.ProfilePost{
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
		},
	}
	if add {
		info.ProfileChanges[0].Profile = testProfile
	}
	return info
}

func (s *mutaterSuite) TestVerifyCurrentProfilesTrue(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, profiles)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseLength(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	profiles := []string{"default", "juju-testme", "juju-testme-lxd-profile-0"}
	s.expectLXDProfileNames(profiles, nil)

	ok, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, append(profiles, "juju-testme-next-1"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesFalseContents(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.expectLXDProfileNames([]string{"default", "juju-testme", "juju-testme-lxd-profile-0"}, nil)

	ok, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default", "juju-testme", "juju-testme-lxd-profile-1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *mutaterSuite) TestVerifyCurrentProfilesError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.expectLXDProfileNames([]string{}, errors.NotFoundf("instId"))

	ok, err := instancemutater.VerifyCurrentProfiles(s.mutaterMachine, s.instId, []string{"default"})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(ok, jc.IsFalse)
}

func (s *mutaterSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	logger := loggo.GetLogger("mutaterSuite")

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
	mExp.Refresh().Return(nil)
	mExp.Life().Return(life.Alive)
	mExp.SetModificationStatus(status.Idle, "", nil).Return(nil)
}

func (s *mutaterSuite) expectRefreshLifeDead() {
	mExp := s.machine.EXPECT()
	mExp.Refresh().Return(nil)
	mExp.Life().Return(life.Dead)
}

func (s *mutaterSuite) expectModificationStatusApplied() {
	s.machine.EXPECT().SetModificationStatus(status.Applied, "", nil).Return(nil)
}

func (s *mutaterSuite) expectModificationStatusError() {
	s.machine.EXPECT().SetModificationStatus(status.Error, gomock.Any(), gomock.Any()).Return(nil)
}

func (s *mutaterSuite) expectAssignLXDProfiles(profiles []string, err error) {
	s.broker.EXPECT().AssignLXDProfiles(s.instId, profiles, gomock.Any()).Return(profiles, err)
}

func (s *mutaterSuite) expectSetCharmProfiles(profiles []string) {
	s.machine.EXPECT().SetCharmProfiles(profiles)
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
