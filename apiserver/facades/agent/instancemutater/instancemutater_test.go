// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	coretesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facades/agent/instancemutater"
	"github.com/juju/juju/apiserver/facades/agent/instancemutater/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type InstanceMutaterAPISuite struct {
	coretesting.IsolationSuite

	state      *mocks.MockInstanceMutaterState
	resources  *mocks.MockResources
	authorizer *mocks.MockAuthorizer
	entity     *mocks.MockEntity
	lifer      *mocks.MockLifer
}

var _ = gc.Suite(&InstanceMutaterAPISuite{})

func (s *InstanceMutaterAPISuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = mocks.NewMockInstanceMutaterState(ctrl)
	s.resources = mocks.NewMockResources(ctrl)
	s.authorizer = mocks.NewMockAuthorizer(ctrl)
	s.entity = mocks.NewMockEntity(ctrl)
	s.lifer = mocks.NewMockLifer(ctrl)

	return ctrl
}

func (s *InstanceMutaterAPISuite) TestLife(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag)

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

func (s *InstanceMutaterAPISuite) TestLifeWithInvalidType(c *gc.C) {
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

func (s *InstanceMutaterAPISuite) TestLifeWithParentId(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("0/lxd/0")

	s.authorizer.EXPECT().AuthMachineAgent().Return(true)
	s.behaviourLife(machineTag)
	s.behaviourFindEntity(machineTag)

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

func (s *InstanceMutaterAPISuite) TestLifeWithInvalidParentId(c *gc.C) {
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

func (s *InstanceMutaterAPISuite) behaviourLife(machineTag names.Tag) {
	exp := s.authorizer.EXPECT()
	gomock.InOrder(
		exp.AuthController().Return(true),
		exp.AuthMachineAgent().Return(true),
		exp.GetAuthTag().Return(machineTag),
	)
}

func (s *InstanceMutaterAPISuite) behaviourFindEntity(machineTag names.Tag) {
	entity := entityShim{
		Entity: s.entity,
		Lifer:  s.lifer,
	}
	s.state.EXPECT().FindEntity(machineTag).Return(entity, nil)
	s.lifer.EXPECT().Life().Return(state.Alive)
}

type entityShim struct {
	state.Entity
	state.Lifer
}
