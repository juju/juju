// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
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
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) behaviourMachine(id instance.Id) {
	s.machine.EXPECT().InstanceId().Return(id, nil)
}

func (s *InstanceMutaterAPICharmProfilingInfoSuite) behaviourFindEntity(machineTag names.Tag, entity state.Entity) {
	s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
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

type machineEntityShim struct {
	instancemutater.Machine
	state.Entity
	state.Lifer
}
