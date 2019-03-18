// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
)

type instanceMutaterAPISuite struct {
	coretesting.IsolationSuite

	authorizer *mocks.MockAuthorizer
	entity     *mocks.MockEntity
	lifer      *mocks.MockLifer
	state      *mocks.MockInstanceMutaterState
	resources  *mocks.MockResources
}

func (s *instanceMutaterAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = mocks.NewMockAuthorizer(ctrl)
	s.entity = mocks.NewMockEntity(ctrl)
	s.lifer = mocks.NewMockLifer(ctrl)
	s.state = mocks.NewMockInstanceMutaterState(ctrl)
	s.resources = mocks.NewMockResources(ctrl)

	return ctrl
}

func (s *instanceMutaterAPISuite) behaviourLife(machineTag names.Tag) {
	exp := s.authorizer.EXPECT()
	gomock.InOrder(
		exp.AuthController().Return(true),
		exp.AuthMachineAgent().Return(true),
		exp.GetAuthTag().Return(machineTag),
	)
}

func (s *instanceMutaterAPISuite) behaviourFindEntity(machineTag names.Tag, entity state.Entity) {
	s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
}

type InstanceMutaterAPILifeSuite struct {
	instanceMutaterAPISuite
}

var _ = gc.Suite(&InstanceMutaterAPILifeSuite{})

func (s *InstanceMutaterAPILifeSuite) TestLife(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, entityShim{
		Entity: s.entity,
		Lifer:  s.lifer,
	})

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: params.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidType(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "user-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access",
				},
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, entityShim{
		Entity: s.entity,
		Lifer:  s.lifer,
	})

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0-lxd-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Life: params.Alive,
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) TestLifeWithInvalidParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.Life(params.Entities{
		Entities: []params.Entity{{Tag: "machine-1-lxd-0"}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access",
				},
			},
		},
	})
}

func (s *InstanceMutaterAPILifeSuite) behaviourFindEntity(machineTag names.Tag, entity state.Entity) {
	s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)
}

type entityShim struct {
	state.Entity
	state.Lifer
}

type InstanceMutaterAPICharmProfilingInfoSuite struct {
	instanceMutaterAPISuite

	machine     *mocks.MockMachine
	model       *mocks.MockModel
	unit        *mocks.MockUnit
	application *mocks.MockApplication
	charm       *mocks.MockCharm
	lxdProfile  *mocks.MockLXDProfile
}

var _ = gc.Suite(&InstanceMutaterAPICharmProfilingInfoSuite{})

func (s *InstanceMutaterAPICharmProfilingInfoSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.model = mocks.NewMockModel(ctrl)
	s.unit = mocks.NewMockUnit(ctrl)
	s.application = mocks.NewMockApplication(ctrl)
	s.charm = mocks.NewMockCharm(ctrl)
	s.lxdProfile = mocks.NewMockLXDProfile(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfo(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	unitNames := []string{"unit"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourMachine(instance.Id("0"))
	s.behaviourCharmProfiles()

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.ProfileChanges, gc.HasLen, 1)
	c.Assert(results.CurrentProfiles, gc.HasLen, 1)
	c.Assert(results.Changes, jc.IsTrue)
	c.Assert(results.ProfileChanges, gc.DeepEquals, []params.ProfileChangeResult{
		{
			OldProfileName: "",
			NewProfileName: "juju-foo-app-0",
			Profile: &params.CharmLXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "dummy profile description",
				Devices: map[string]map[string]string{
					"tun": {
						"path": "/dev/net/tun",
					},
				},
			},
			Subordinate: false,
		},
	})
	c.Assert(results.CurrentProfiles, gc.DeepEquals, []string{
		"charm-foo-0",
	})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithNoProfile(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	unitNames := []string{"unit", "empty"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourMachine(instance.Id("0"))
	s.behaviourCharmProfiles()
	s.behaviourCharmProfilesWithEmpty()

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.ProfileChanges, gc.HasLen, 2)
	c.Assert(results.CurrentProfiles, gc.HasLen, 1)
	c.Assert(results.Changes, jc.IsTrue)
	c.Assert(results.ProfileChanges, gc.DeepEquals, []params.ProfileChangeResult{
		{
			OldProfileName: "",
			NewProfileName: "juju-foo-app-0",
			Profile: &params.CharmLXDProfile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "dummy profile description",
				Devices: map[string]map[string]string{
					"tun": {
						"path": "/dev/net/tun",
					},
				},
			},
			Subordinate: false,
		},
		{},
	})
	c.Assert(results.CurrentProfiles, gc.DeepEquals, []string{
		"charm-foo-0",
	})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithInvalidMachine(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	unitNames := []string{"unit"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.state.EXPECT().FindEntity(machineTag).Return(s.entity, errors.New("not found"))

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "not found")
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithMachineNotProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	unitNames := []string{"unit"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.machine.EXPECT().InstanceId().Return(instance.Id("0"), params.Error{Code: params.CodeNotProvisioned})

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.ProfileChanges, gc.HasLen, 0)
	c.Assert(results.CurrentProfiles, gc.HasLen, 0)
	c.Assert(results.Changes, jc.IsFalse)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) behaviourMachine(id instance.Id) {
	s.machine.EXPECT().InstanceId().Return(id, nil)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) behaviourCharmProfiles() {
	aExp := s.application.EXPECT()
	cExp := s.charm.EXPECT()
	dExp := s.model.EXPECT()
	lExp := s.lxdProfile.EXPECT()
	mExp := s.machine.EXPECT()
	sExp := s.state.EXPECT()
	uExp := s.unit.EXPECT()

	sExp.Model().Return(s.model, nil)
	dExp.Name().Return("foo")

	machineProfiles := []string{
		"charm-foo-0",
	}
	profileConfig := map[string]string{
		"security.nesting": "true",
	}
	profileDescription := "dummy profile description"
	profileDevices := map[string]map[string]string{
		"tun": {
			"path": "/dev/net/tun",
		},
	}

	mExp.CharmProfiles().Return(machineProfiles, nil)
	sExp.Unit("unit").Return(s.unit, nil)
	uExp.Application().Return(s.application, nil)
	aExp.Charm().Return(s.charm, nil)
	cExp.LXDProfile().Return(s.lxdProfile).Times(2)
	lExp.Empty().Return(false)
	lExp.Config().Return(profileConfig)
	lExp.Description().Return(profileDescription)
	lExp.Devices().Return(profileDevices)
	aExp.Name().Return("app")
	cExp.Revision().Return(0)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) behaviourCharmProfilesWithEmpty() {
	aExp := s.application.EXPECT()
	cExp := s.charm.EXPECT()
	lExp := s.lxdProfile.EXPECT()
	sExp := s.state.EXPECT()
	uExp := s.unit.EXPECT()

	sExp.Unit("empty").Return(s.unit, nil)
	uExp.Application().Return(s.application, nil)
	aExp.Charm().Return(s.charm, nil)
	cExp.LXDProfile().Return(s.lxdProfile).Times(2)
	lExp.Empty().Return(true)
	aExp.Name().Return("app")
	cExp.Revision().Return(0)
}

type machineEntityShim struct {
	instancemutater.Machine
	state.Entity
	state.Lifer
}

type InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
}

var _ = gc.Suite(&InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite{})

func (s *InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite) TestSetUpgradeCharmProfileComplete(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourSetProfileMsg(lxdprofile.SuccessStatus, nil)

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.SetUpgradeCharmProfileComplete(params.SetProfileUpgradeCompleteArgs{
		Args: []params.SetProfileUpgradeCompleteArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				UnitName: "unit",
				Message:  lxdprofile.SuccessStatus,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{}})
}

func (s *InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite) TestSetUpgradeCharmProfileCompleteWithError(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourSetProfileMsg(lxdprofile.SuccessStatus, nil)
	s.behaviourSetProfileMsg(lxdprofile.SuccessStatus, errors.New("Failure"))

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.SetUpgradeCharmProfileComplete(params.SetProfileUpgradeCompleteArgs{
		Args: []params.SetProfileUpgradeCompleteArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				UnitName: "unit",
				Message:  lxdprofile.SuccessStatus,
			},
			{
				Entity:   params.Entity{Tag: "machine-0"},
				UnitName: "unit",
				Message:  lxdprofile.SuccessStatus,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
		{
			Error: &params.Error{
				Message: "Failure",
			},
		},
	})
}

func (s *InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite) behaviourSetProfileMsg(msg string, err error) {
	s.machine.EXPECT().SetUpgradeCharmProfileComplete("unit", msg).Return(err)
}

type InstanceMutaterAPISetCharmProfilesSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
}

var _ = gc.Suite(&InstanceMutaterAPISetCharmProfilesSuite{})

func (s *InstanceMutaterAPISetCharmProfilesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) TestSetCharmProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	profiles := []string{"unit-foo-0"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourSetProfiles(profiles, nil)

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.SetCharmProfiles(params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{}})
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) TestSetCharmProfilesWithError(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")
	profiles := []string{"unit-foo-0"}

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourSetProfiles(profiles, nil)
	s.behaviourFindEntity(machineTag, machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
		Lifer:   s.lifer,
	})
	s.behaviourSetProfiles(profiles, errors.New("Failure"))

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)

	results, err := facade.SetCharmProfiles(params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
			{
				Entity:   params.Entity{Tag: "machine-0"},
				Profiles: profiles,
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{
		{},
		{
			Error: &params.Error{
				Message: "Failure",
			},
		},
	})
}

func (s *InstanceMutaterAPISetCharmProfilesSuite) behaviourSetProfiles(profiles []string, err error) {
	s.machine.EXPECT().SetCharmProfiles(profiles).Return(err)
}
