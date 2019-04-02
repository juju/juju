// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type instanceMutaterAPISuite struct {
	coretesting.IsolationSuite

	authorizer *mocks.MockAuthorizer
	entity     *mocks.MockEntity
	lifer      *mocks.MockLifer
	state      *mocks.MockInstanceMutaterState
	model      *mocks.MockModelCache
	resources  *mocks.MockResources

	machineTag names.Tag
	done       chan struct{}
}

func (s *instanceMutaterAPISuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.machineTag = names.NewMachineTag("0")
	s.done = make(chan struct{})
}

func (s *instanceMutaterAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = mocks.NewMockAuthorizer(ctrl)
	s.entity = mocks.NewMockEntity(ctrl)
	s.lifer = mocks.NewMockLifer(ctrl)
	s.state = mocks.NewMockInstanceMutaterState(ctrl)
	s.model = mocks.NewMockModelCache(ctrl)
	s.resources = mocks.NewMockResources(ctrl)

	return ctrl
}

func (s *instanceMutaterAPISuite) facadeAPIForScenario(c *gc.C, behaviours ...func()) *instancemutater.InstanceMutaterAPI {
	for _, b := range behaviours {
		b()
	}

	facade, err := instancemutater.NewInstanceMutaterAPI(s.state, s.model, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	return facade
}

func (s *instanceMutaterAPISuite) expectLife(machineTag names.Tag) func() {
	return func() {
		exp := s.authorizer.EXPECT()
		gomock.InOrder(
			exp.AuthController().Return(true),
			exp.AuthMachineAgent().Return(true),
			exp.GetAuthTag().Return(machineTag),
		)
	}
}

func (s *instanceMutaterAPISuite) expectFindEntity(machineTag names.Tag, entity state.Entity) func() {
	return func() {
		s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
	}
}

func (s *instanceMutaterAPISuite) expectAuthMachineAgent() {
	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
}

func (s *instanceMutaterAPISuite) assertStop(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
}

type InstanceMutaterAPILifeSuite struct {
	instanceMutaterAPISuite
}

var _ = gc.Suite(&InstanceMutaterAPILifeSuite{})

func (s *InstanceMutaterAPILifeSuite) TestLife(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, entityShim{
			Entity: s.entity,
			Lifer:  s.lifer,
		}),
	)

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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
	)

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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(machineTag),
		s.expectFindEntity(machineTag, entityShim{
			Entity: s.entity,
			Lifer:  s.lifer,
		}),
	)

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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(machineTag),
	)

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

func (s *InstanceMutaterAPILifeSuite) expectFindEntity(machineTag names.Tag, entity state.Entity) func() {
	return func() {
		s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
		s.lifer.EXPECT().Life().Return(state.Alive)
	}
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

	unitNames := []string{"unit"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectMachine(instance.Id("0")),
		s.expectCharmProfiles,
	)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.InstanceId, gc.Equals, instance.Id("0"))
	c.Assert(results.ModelName, gc.Equals, "foo")
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

	unitNames := []string{"unit", "empty"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectMachine(instance.Id("0")),
		s.expectCharmProfiles,
		s.expectCharmProfilesWithEmpty,
	)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.InstanceId, gc.Equals, instance.Id("0"))
	c.Assert(results.ModelName, gc.Equals, "foo")
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

	unitNames := []string{"unit"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntityWithNotFoundError,
	)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.ErrorMatches, "not found")
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) TestCharmProfilingInfoWithMachineNotProvisioned(c *gc.C) {
	defer s.setup(c).Finish()

	unitNames := []string{"unit"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectInstanceIdNotProvisioned,
	)

	results, err := facade.CharmProfilingInfo(params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: "machine-0"},
		UnitNames: unitNames,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.InstanceId, gc.Equals, instance.Id(""))
	c.Assert(results.ModelName, gc.Equals, "")
	c.Assert(results.ProfileChanges, gc.HasLen, 0)
	c.Assert(results.CurrentProfiles, gc.HasLen, 0)
	c.Assert(results.Changes, jc.IsFalse)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectMachine(id instance.Id) func() {
	return func() {
		s.machine.EXPECT().InstanceId().Return(id, nil)
	}
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectFindEntityWithNotFoundError() {
	s.state.EXPECT().FindEntity(s.machineTag).Return(s.entity, errors.New("not found"))
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectInstanceIdNotProvisioned() {
	s.machine.EXPECT().InstanceId().Return(instance.Id("0"), params.Error{Code: params.CodeNotProvisioned})
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectCharmProfiles() {
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

func (s *InstanceMutaterAPICharmProfilingInfoSuite) expectCharmProfilesWithEmpty() {
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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfileMsg(lxdprofile.SuccessStatus, nil),
	)

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

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfileMsg(lxdprofile.SuccessStatus, nil),
		s.expectSetProfileMsg(lxdprofile.SuccessStatus, errors.New("Failure")),
	)

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

func (s *InstanceMutaterAPISetUpgradeCharmProfileCompleteSuite) expectSetProfileMsg(msg string, err error) func() {
	return func() {
		s.machine.EXPECT().SetUpgradeCharmProfileComplete("unit", msg).Return(err)
	}
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

	profiles := []string{"unit-foo-0"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, nil),
	)

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

	profiles := []string{"unit-foo-0"}

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, nil),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetProfiles(profiles, errors.New("Failure")),
	)

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

func (s *InstanceMutaterAPISetCharmProfilesSuite) expectSetProfiles(profiles []string, err error) func() {
	return func() {
		s.machine.EXPECT().SetCharmProfiles(profiles).Return(err)
	}
}

type InstanceMutaterAPISetModificationStatusSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
}

var _ = gc.Suite(&InstanceMutaterAPISetModificationStatusSuite{})

func (s *InstanceMutaterAPISetModificationStatusSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISetModificationStatusSuite) TestSetModificationStatusProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetModificationStatus(status.Applied, "applied", nil),
	)

	result, err := facade.SetModificationStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-0", Status: "applied", Info: "applied", Data: nil},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
		},
	})
}

func (s *InstanceMutaterAPISetModificationStatusSuite) TestSetModificationStatusProfilesWithError(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectFindEntity(s.machineTag, machineEntityShim{
			Machine: s.machine,
			Entity:  s.entity,
			Lifer:   s.lifer,
		}),
		s.expectSetModificationStatus(status.Applied, "applied", errors.New("failed")),
	)

	result, err := facade.SetModificationStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "machine-0", Status: "applied", Info: "applied", Data: nil},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: "failed"}},
		},
	})
}

func (s *InstanceMutaterAPISetModificationStatusSuite) expectSetModificationStatus(st status.Status, message string, err error) func() {
	return func() {
		now := time.Now()

		sExp := s.state.EXPECT()
		sExp.ControllerTimestamp().Return(&now, nil)

		mExp := s.machine.EXPECT()
		mExp.SetModificationStatus(status.StatusInfo{
			Status:  st,
			Message: message,
			Data:    nil,
			Since:   &now,
		}).Return(err)
	}
}

type InstanceMutaterAPIWatchMachinesSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockMachine
	watcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchMachinesSuite{})

func (s *InstanceMutaterAPIWatchMachinesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockMachine(ctrl)
	s.watcher = mocks.NewMockStringsWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachines(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectAuthController,
		s.expectWatchMachinesWithNotify(1),
	)

	result, err := facade.WatchMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"0"},
	})
	s.assertStop(c)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) TestWatchMachinesWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectAuthController,
		s.expectWatchMachinesWithClosedChannel,
	)

	_, err := facade.WatchMachines()
	c.Assert(err, gc.ErrorMatches, "cannot obtain initial model machines")
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithNotify(times int) func() {
	return func() {
		ch := make(chan []string)

		go func() {
			for i := 0; i < times; i++ {
				ch <- []string{fmt.Sprintf("%d", i)}
			}
			close(s.done)
		}()

		s.model.EXPECT().WatchMachines().Return(s.watcher)
		s.watcher.EXPECT().Changes().Return(ch)
		s.resources.EXPECT().Register(s.watcher).Return("1")
	}
}

func (s *InstanceMutaterAPIWatchMachinesSuite) expectWatchMachinesWithClosedChannel() {
	ch := make(chan []string)
	close(ch)

	s.model.EXPECT().WatchMachines().Return(s.watcher)
	s.watcher.EXPECT().Changes().Return(ch)
}

type InstanceMutaterAPIWatchApplicationLXDProfilesSuite struct {
	instanceMutaterAPISuite

	machine *mocks.MockModelCacheMachine
	watcher *mocks.MockNotifyWatcher
}

var _ = gc.Suite(&InstanceMutaterAPIWatchApplicationLXDProfilesSuite{})

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := s.instanceMutaterAPISuite.setup(c)

	s.machine = mocks.NewMockModelCacheMachine(ctrl)
	s.watcher = mocks.NewMockNotifyWatcher(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) TestWatchApplicationLXDProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchApplicationLXDProfilesWithNotify(1),
	)

	result, err := facade.WatchApplicationLXDProfiles(params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}},
	})
	s.assertStop(c)
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) TestWatchApplicationLXDProfilesWithInvalidTag(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
	)

	result, err := facade.WatchApplicationLXDProfiles(params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("bob@local").String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(common.ErrPerm),
		}},
	})
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) TestWatchApplicationLXDProfilesWithClosedChannel(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchApplicationLXDProfilesWithClosedChannel,
	)

	result, err := facade.WatchApplicationLXDProfiles(params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(errors.New("cannot obtain initial machine watch application LXD profiles")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) TestWatchApplicationLXDProfilesModelCacheError(c *gc.C) {
	defer s.setup(c).Finish()

	facade := s.facadeAPIForScenario(c,
		s.expectAuthMachineAgent,
		s.expectLife(s.machineTag),
		s.expectWatchApplicationLXDProfilesError,
	)

	result, err := facade.WatchApplicationLXDProfiles(params.Entities{
		Entities: []params.Entity{{Tag: s.machineTag.String()}},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			Error: common.ServerError(errors.New("error from model cache")),
		}},
	})
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) expectAuthController() {
	s.authorizer.EXPECT().AuthController().Return(true)
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) expectWatchApplicationLXDProfilesWithNotify(times int) func() {
	return func() {
		ch := make(chan struct{})

		go func() {
			for i := 0; i < times; i++ {
				ch <- struct{}{}
			}
			close(s.done)
		}()

		s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
		s.machine.EXPECT().WatchApplicationLXDProfiles().Return(s.watcher, nil)
		s.watcher.EXPECT().Changes().Return(ch)
		s.resources.EXPECT().Register(s.watcher).Return("1")
	}
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) expectWatchApplicationLXDProfilesWithClosedChannel() {
	ch := make(chan struct{})
	close(ch)

	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchApplicationLXDProfiles().Return(s.watcher, nil)
	s.watcher.EXPECT().Changes().Return(ch)
}

func (s *InstanceMutaterAPIWatchApplicationLXDProfilesSuite) expectWatchApplicationLXDProfilesError() {
	s.model.EXPECT().Machine(s.machineTag.Id()).Return(s.machine, nil)
	s.machine.EXPECT().WatchApplicationLXDProfiles().Return(s.watcher, errors.New("error from model cache"))
}
