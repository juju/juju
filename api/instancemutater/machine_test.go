// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/api/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	jujutesting "github.com/juju/juju/testing"
)

type instanceMutaterMachineSuite struct {
	jujutesting.BaseSuite

	args     params.Entities
	message  string
	tag      names.MachineTag
	unitName string

	fCaller   *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICaller
}

var _ = gc.Suite(&instanceMutaterMachineSuite{})

func (s *instanceMutaterMachineSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}
	s.unitName = "lxd-profile/0"
	s.message = lxdprofile.SuccessStatus
	s.BaseSuite.SetUpTest(c)
}

func (s *instanceMutaterMachineSuite) TestSetUpgradeCharmProfileCompleteSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetUpgradeCharmProfileCompleteFacadeCall,
	)

	err := m.SetUpgradeCharmProfileComplete(s.unitName, s.message)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestSetUpgradeCharmProfileCompleteError(c *gc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetUpgradeCharmProfileCompleteFacadeCallReturnsError(errors.New("failed")),
	)

	err := m.SetUpgradeCharmProfileComplete(s.unitName, s.message)
	c.Assert(err, gc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestSetUpgradeCharmProfileCompleteErrorExpectedOne(c *gc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetUpgradeCharmProfileCompleteFacadeCallReturnsTwoErrors,
	)

	err := m.SetUpgradeCharmProfileComplete(s.unitName, s.message)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *instanceMutaterMachineSuite) TestWatchUnitsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	results := params.StringsWatchResults{Results: []params.StringsWatchResult{{
		StringsWatcherId: "1",
		Error:            nil,
	}}}

	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("StringsWatcher").Return(1)
	aExp.APICall("StringsWatcher", 1, "1", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("WatchUnits", s.args, gomock.Any()).SetArg(2, results).Return(nil)
	fExp.RawAPICaller().Return(s.apiCaller)

	ch, err := s.setupMachine().WatchUnits()
	c.Assert(err, jc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-ch.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}

func (s *instanceMutaterMachineSuite) TestWatchApplicationLXDProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchApplicationLXDProfiles,
		s.expectNotifyWatcher,
	)
	ch, err := api.WatchApplicationLXDProfiles()
	c.Assert(err, jc.ErrorIsNil)

	// watch for the changes
	for i := 0; i < 2; i++ {
		select {
		case <-ch.Changes():
		case <-time.After(jujutesting.LongWait):
			c.Fail()
		}
	}
}

func (s *instanceMutaterMachineSuite) TestWatchApplicationLXDProfilesServerError(c *gc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchApplicationLXDProfilesWithError(errors.New("failed")),
	)
	_, err := api.WatchApplicationLXDProfiles()
	c.Assert(err, gc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessChanges(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: s.tag.String()},
		UnitNames: []string{s.unitName},
	}
	results := params.CharmProfilingInfoResult{
		Changes:         true,
		CurrentProfiles: []string{"juju-default-neutron-ovswitch-255"},
		Error:           nil,
		ProfileChanges: []params.ProfileChangeResult{{
			OldProfileName: "",
			NewProfileName: "juju-default-lxd-profile-3",
			Profile: &params.CharmLXDProfile{
				Description: "Test Profile",
			},
			Subordinate: true,
		}},
	}

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("CharmProfilingInfo", args, gomock.Any()).SetArg(2, results).Return(nil)

	info, err := s.setupMachine().CharmProfilingInfo([]string{s.unitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Changes, jc.IsTrue)
	c.Assert(info.CurrentProfiles, gc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges[0].OldProfileName, gc.Equals, results.ProfileChanges[0].OldProfileName)
	c.Assert(info.ProfileChanges[0].NewProfileName, gc.Equals, results.ProfileChanges[0].NewProfileName)
	c.Assert(info.ProfileChanges[0].Profile.Description, gc.Equals, "Test Profile")
	c.Assert(info.ProfileChanges[0].Subordinate, jc.IsTrue)
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessNoChanges(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: s.tag.String()},
		UnitNames: []string{s.unitName},
	}
	results := params.CharmProfilingInfoResult{
		Changes:         false,
		CurrentProfiles: []string{"juju-default-neutron-ovswitch-255"},
		Error:           nil,
		ProfileChanges: []params.ProfileChangeResult{{
			NewProfileName: "juju-default-lxd-profile-3", // including to make sure it's not copied over.
		}},
	}

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("CharmProfilingInfo", args, gomock.Any()).SetArg(2, results).Return(nil)

	info, err := s.setupMachine().CharmProfilingInfo([]string{s.unitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Changes, jc.IsFalse)
	c.Assert(info.CurrentProfiles, gc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges, gc.HasLen, 0)
}

func (s *instanceMutaterMachineSuite) TestSetModificationStatus(c *gc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetModificationFacadeCall(status.Applied, "applied", nil),
	)

	err := m.SetModificationStatus(status.Applied, "applied", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestSetModificationStatusReturnsError(c *gc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetModificationFacadeCallReturnsError(errors.New("bad")),
	)

	err := m.SetModificationStatus(status.Applied, "applied", nil)
	c.Assert(err, gc.ErrorMatches, "bad")
}

func (s *instanceMutaterMachineSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func (s *instanceMutaterMachineSuite) setUpSetProfileUpgradeCompleteArgs() params.SetProfileUpgradeCompleteArgs {
	return params.SetProfileUpgradeCompleteArgs{
		Args: []params.SetProfileUpgradeCompleteArg{
			{
				Entity:   params.Entity{Tag: s.tag.String()},
				UnitName: s.unitName,
				Message:  s.message,
			},
		},
	}
}

func (s *instanceMutaterMachineSuite) setupMachine() *instancemutater.Machine {
	return instancemutater.NewMachine(s.fCaller, s.tag, params.Alive)
}

func (s *instanceMutaterMachineSuite) machineForScenario(c *gc.C, behaviours ...func()) *instancemutater.Machine {
	for _, b := range behaviours {
		b()
	}

	return s.setupMachine()
}

func (s *instanceMutaterMachineSuite) expectSetUpgradeCharmProfileCompleteFacadeCall() {
	results := params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
	args := s.setUpSetProfileUpgradeCompleteArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)
}

func (s *instanceMutaterMachineSuite) expectSetUpgradeCharmProfileCompleteFacadeCallReturnsError(err error) func() {
	return func() {
		results := params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: err.Error()}},
			},
		}
		args := s.setUpSetProfileUpgradeCompleteArgs()

		fExp := s.fCaller.EXPECT()
		fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectSetUpgradeCharmProfileCompleteFacadeCallReturnsTwoErrors() {
	results := params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
		},
	}
	args := s.setUpSetProfileUpgradeCompleteArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)
}

func (s *instanceMutaterMachineSuite) expectSetModificationFacadeCall(status status.Status, info string, data map[string]interface{}) func() {
	return func() {
		args := params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: s.tag.String(), Status: status.String(), Info: info, Data: data},
			},
		}
		results := params.ErrorResults{
			Results: []params.ErrorResult{
				{},
			},
		}

		fExp := s.fCaller.EXPECT()
		fExp.FacadeCall("SetModificationStatus", args, gomock.Any()).SetArg(2, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectSetModificationFacadeCallReturnsError(err error) func() {
	return func() {
		args := params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: s.tag.String(), Status: status.Applied.String(), Info: "applied"},
			},
		}
		results := params.ErrorResults{
			Results: []params.ErrorResult{
				{
					Error: &params.Error{
						Message: err.Error(),
					},
				},
			},
		}

		fExp := s.fCaller.EXPECT()
		fExp.FacadeCall("SetModificationStatus", args, gomock.Any()).SetArg(2, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectWatchApplicationLXDProfiles() {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.tag.String()},
		},
	}
	results := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{
				NotifyWatcherId: "1",
			},
		},
	}
	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("WatchApplicationLXDProfiles", args, gomock.Any()).SetArg(2, results).Return(nil)
	fExp.RawAPICaller().Return(s.apiCaller)
}

func (s *instanceMutaterMachineSuite) expectWatchApplicationLXDProfilesWithError(err error) func() {
	return func() {
		args := params.Entities{
			Entities: []params.Entity{
				{Tag: s.tag.String()},
			},
		}
		results := params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{
				{
					Error: &params.Error{
						Message: err.Error(),
					},
				},
			},
		}
		aExp := s.fCaller.EXPECT()
		aExp.FacadeCall("WatchApplicationLXDProfiles", args, gomock.Any()).SetArg(2, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectNotifyWatcher() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("NotifyWatcher").Return(1)
	aExp.APICall("NotifyWatcher", 1, "1", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)
}
