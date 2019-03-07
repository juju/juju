// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancemutater"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lxdprofile"
	jujutesting "github.com/juju/juju/testing"
)

type instanceMutaterMachineSuite struct {
	jujutesting.BaseSuite

	args     params.Entities
	message  string
	tag      names.MachineTag
	unitName string

	fCaller *mocks.MockFacadeCaller
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
	defer s.setUpMocks(c).Finish()

	results := params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
	args := s.setUpSetProfileUpgradeCompleteArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)

	err := s.setUpMachine().SetUpgradeCharmProfileComplete(s.unitName, s.message)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestSetUpgradeCharmProfileCompleteError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	results := params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: "failed"}}}}
	args := s.setUpSetProfileUpgradeCompleteArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)

	err := s.setUpMachine().SetUpgradeCharmProfileComplete(s.unitName, s.message)
	c.Assert(err, gc.ErrorMatches, "failed")
}

func (s *instanceMutaterMachineSuite) TestSetUpgradeCharmProfileCompleteErrorExpectedOne(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	message := lxdprofile.SuccessStatus
	results := params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}}
	args := s.setUpSetProfileUpgradeCompleteArgs()

	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("SetUpgradeCharmProfileComplete", args, gomock.Any()).SetArg(2, results).Return(nil)

	err := s.setUpMachine().SetUpgradeCharmProfileComplete(s.unitName, message)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *instanceMutaterMachineSuite) TestWatchUnitsSuccess(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	results := params.StringsWatchResults{Results: []params.StringsWatchResult{{
		StringsWatcherId: "1",
		Error:            nil,
	}}}
	apiCaller := apitesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "StringsWatcher")
			c.Check(id, gc.Equals, "1")
			c.Check(request, gc.Equals, "Next")
			c.Check(a, gc.IsNil)
			return nil
		},
	)
	rawAPICaller := apitesting.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	fExp := s.fCaller.EXPECT()
	fExp.FacadeCall("WatchUnits", s.args, gomock.Any()).SetArg(2, results).Return(nil)
	fExp.RawAPICaller().Return(rawAPICaller)

	_, err := s.setUpMachine().WatchUnits()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessChanges(c *gc.C) {
	defer s.setUpMocks(c).Finish()

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

	info, err := s.setUpMachine().CharmProfilingInfo([]string{s.unitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Changes, jc.IsTrue)
	c.Assert(info.CurrentProfiles, gc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges[0].OldProfileName, gc.Equals, results.ProfileChanges[0].OldProfileName)
	c.Assert(info.ProfileChanges[0].NewProfileName, gc.Equals, results.ProfileChanges[0].NewProfileName)
	c.Assert(info.ProfileChanges[0].Profile.Description, gc.Equals, "Test Profile")
	c.Assert(info.ProfileChanges[0].Subordinate, jc.IsTrue)
}

func (s *instanceMutaterMachineSuite) TestCharmProfilingInfoSuccessNoChanges(c *gc.C) {
	defer s.setUpMocks(c).Finish()

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

	info, err := s.setUpMachine().CharmProfilingInfo([]string{s.unitName})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Changes, jc.IsFalse)
	c.Assert(info.CurrentProfiles, gc.DeepEquals, results.CurrentProfiles)
	c.Assert(info.ProfileChanges, gc.HasLen, 0)
}

func (s *instanceMutaterMachineSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)

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

func (s *instanceMutaterMachineSuite) setUpMachine() *instancemutater.Machine {
	return instancemutater.NewMachine(s.fCaller, s.tag, params.Alive)
}
