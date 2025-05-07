// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/api/agent/instancemutater/mocks"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type instanceMutaterMachineSuite struct {
	jujutesting.BaseSuite

	args     params.Entities
	message  string
	tag      names.MachineTag
	unitName string
	profiles []string

	fCaller   *mocks.MockFacadeCaller
	apiCaller *mocks.MockAPICaller
}

var _ = tc.Suite(&instanceMutaterMachineSuite{})

func (s *instanceMutaterMachineSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}
	s.unitName = "lxd-profile/0"
	s.profiles = []string{"charm-app-x-0", "charm-app-y-1"}
	s.message = lxdprofile.SuccessStatus
	s.BaseSuite.SetUpTest(c)
}

func (s *instanceMutaterMachineSuite) TestSetCharmProfiles(c *tc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetCharmProfilesFacadeCall,
	)

	err := m.SetCharmProfiles(context.Background(), s.profiles)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestSetCharmProfilesError(c *tc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetCharmProfilesFacadeCallReturnsError(errors.New("failed")),
	)

	err := m.SetCharmProfiles(context.Background(), s.profiles)
	c.Assert(err, tc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestWatchLXDProfileVerificationNeeded(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchLXDProfileVerificationNeeded,
		s.expectNotifyWatcher,
	)
	ch, err := api.WatchLXDProfileVerificationNeeded(context.Background())
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

func (s *instanceMutaterMachineSuite) TestWatchLXDProfileVerificationNeededServerError(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchLXDProfileVerificationNeededWithError("", "failed"),
	)
	_, err := api.WatchLXDProfileVerificationNeeded(context.Background())
	c.Assert(err, tc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestWatchLXDProfileVerificationNeededNotSupported(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchLXDProfileVerificationNeededWithError("not supported", "failed"),
	)
	_, err := api.WatchLXDProfileVerificationNeeded(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *instanceMutaterMachineSuite) TestWatchContainers(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchContainers,
		s.expectStringsWatcher,
	)
	ch, err := api.WatchContainers(context.Background())
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

func (s *instanceMutaterMachineSuite) TestWatchContainersServerError(c *tc.C) {
	defer s.setup(c).Finish()

	api := s.machineForScenario(c,
		s.expectWatchContainersWithErrors(errors.New("failed")),
	)
	_, err := api.WatchContainers(context.Background())
	c.Assert(err, tc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessChanges(c *tc.C) {
	defer s.setup(c).Finish()

	args := params.Entity{Tag: s.tag.String()}
	results := params.CharmProfilingInfoResult{
		InstanceId:      instance.Id("juju-gd4c23-0"),
		ModelName:       "default",
		CurrentProfiles: []string{"juju-default-neutron-ovswitch-255"},
		Error:           nil,
		ProfileChanges: []params.ProfileInfoResult{{
			Profile: &params.CharmLXDProfile{
				Description: "Test Profile",
			},
		}},
	}

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall(gomock.Any(), "CharmProfilingInfo", args, gomock.Any()).SetArg(3, results).Return(nil)

	info, err := s.setupMachine().CharmProfilingInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.InstanceId, tc.Equals, results.InstanceId)
	c.Assert(info.ModelName, tc.Equals, results.ModelName)
	c.Assert(info.CurrentProfiles, tc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges[0].Profile.Description, tc.Equals, "Test Profile")
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessChangesWithNoProfile(c *tc.C) {
	defer s.setup(c).Finish()

	args := params.Entity{Tag: s.tag.String()}
	results := params.CharmProfilingInfoResult{
		InstanceId:      instance.Id("juju-gd4c23-0"),
		ModelName:       "default",
		CurrentProfiles: []string{"juju-default-neutron-ovswitch-255"},
		Error:           nil,
		ProfileChanges: []params.ProfileInfoResult{{
			Profile: nil,
		}},
	}

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall(gomock.Any(), "CharmProfilingInfo", args, gomock.Any()).SetArg(3, results).Return(nil)

	info, err := s.setupMachine().CharmProfilingInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.InstanceId, tc.Equals, results.InstanceId)
	c.Assert(info.ModelName, tc.Equals, results.ModelName)
	c.Assert(info.CurrentProfiles, tc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges[0].Profile.Description, tc.Equals, "")
}

func (s *instanceMutaterMachineSuite) TestSetModificationStatus(c *tc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetModificationFacadeCall(status.Applied, "applied", nil),
	)

	err := m.SetModificationStatus(context.Background(), status.Applied, "applied", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestSetModificationStatusReturnsError(c *tc.C) {
	defer s.setup(c).Finish()

	m := s.machineForScenario(c,
		s.expectSetModificationFacadeCallReturnsError(errors.New("bad")),
	)

	err := m.SetModificationStatus(context.Background(), status.Applied, "applied", nil)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *instanceMutaterMachineSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.apiCaller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func (s *instanceMutaterMachineSuite) setUpSetCharmProfilesArgs() params.SetProfileArgs {
	return params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: s.tag.String()},
				Profiles: s.profiles,
			},
		},
	}
}

func (s *instanceMutaterMachineSuite) setupMachine() *instancemutater.Machine {
	return instancemutater.NewMachine(s.fCaller, s.tag, life.Alive)
}

func (s *instanceMutaterMachineSuite) machineForScenario(c *tc.C, behaviours ...func()) *instancemutater.Machine {
	for _, b := range behaviours {
		b()
	}

	return s.setupMachine()
}

func (s *instanceMutaterMachineSuite) expectSetCharmProfilesFacadeCall() {
	results := params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
	args := s.setUpSetCharmProfilesArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall(gomock.Any(), "SetCharmProfiles", args, gomock.Any()).SetArg(3, results).Return(nil)
}

func (s *instanceMutaterMachineSuite) expectSetCharmProfilesFacadeCallReturnsError(err error) func() {
	return func() {
		results := params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: err.Error()}},
			},
		}
		args := s.setUpSetCharmProfilesArgs()

		fExp := s.fCaller.EXPECT()
		fExp.FacadeCall(gomock.Any(), "SetCharmProfiles", args, gomock.Any()).SetArg(3, results).Return(nil)
	}
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
		fExp.FacadeCall(gomock.Any(), "SetModificationStatus", args, gomock.Any()).SetArg(3, results).Return(nil)
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
		fExp.FacadeCall(gomock.Any(), "SetModificationStatus", args, gomock.Any()).SetArg(3, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectWatchLXDProfileVerificationNeeded() {
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
	fExp.FacadeCall(gomock.Any(), "WatchLXDProfileVerificationNeeded", args, gomock.Any()).SetArg(3, results).Return(nil)
	fExp.RawAPICaller().Return(s.apiCaller)
}

func (s *instanceMutaterMachineSuite) expectWatchLXDProfileVerificationNeededWithError(code, message string) func() {
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
						Code:    code,
						Message: message,
					},
				},
			},
		}
		aExp := s.fCaller.EXPECT()
		aExp.FacadeCall(gomock.Any(), "WatchLXDProfileVerificationNeeded", args, gomock.Any()).SetArg(3, results).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectWatchContainers() {
	arg := params.Entity{Tag: s.tag.String()}
	result := params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0/lxd/0"},
	}
	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall(gomock.Any(), "WatchContainers", arg, gomock.Any()).SetArg(3, result).Return(nil)
	fExp.RawAPICaller().Return(s.apiCaller)
}

func (s *instanceMutaterMachineSuite) expectWatchContainersWithErrors(err error) func() {
	return func() {
		arg := params.Entity{Tag: s.tag.String()}
		result := params.StringsWatchResult{
			Error: &params.Error{
				Message: err.Error(),
			},
		}
		aExp := s.fCaller.EXPECT()
		aExp.FacadeCall(gomock.Any(), "WatchContainers", arg, gomock.Any()).SetArg(3, result).Return(nil)
	}
}

func (s *instanceMutaterMachineSuite) expectNotifyWatcher() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("NotifyWatcher").Return(1)
	aExp.APICall(gomock.Any(), "NotifyWatcher", 1, "1", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)
}

func (s *instanceMutaterMachineSuite) expectStringsWatcher() {
	aExp := s.apiCaller.EXPECT()
	aExp.BestFacadeVersion("StringsWatcher").Return(1)
	aExp.APICall(gomock.Any(), "StringsWatcher", 1, "1", "Next", nil, gomock.Any()).Return(nil).MinTimes(1)
}
