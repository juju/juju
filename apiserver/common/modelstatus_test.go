// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/controller"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type modelStatusSuite struct {
	statetesting.StateSuite

	controller *controller.ControllerAPI
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&modelStatusSuite{})

func (s *modelStatusSuite) SetUpTest(c *gc.C) {
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})

	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	controller, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      s.authorizer,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = controller

	loggo.GetLogger("juju.apiserver.controller").SetLogLevel(loggo.TRACE)
}

func (s *modelStatusSuite) TestModelStatusNonAuth(c *gc.C) {
	// Set up the user making the call.
	user := s.Factory.MakeUser(c, &factory.UserParams{NoModelUser: true})
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	endpoint, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
		})
	c.Assert(err, jc.ErrorIsNil)
	controllerModelTag := s.State.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerModelTag}},
	}
	_, err = endpoint.ModelStatus(req)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelStatusSuite) TestModelStatusOwnerAllowed(c *gc.C) {
	// Set up the user making the call.
	owner := s.Factory.MakeUser(c, nil)
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: owner.Tag(),
	}
	st := s.Factory.MakeModel(c, &factory.ModelParams{Owner: owner.Tag()})
	defer st.Close()
	endpoint, err := controller.NewControllerAPI(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
		})
	c.Assert(err, jc.ErrorIsNil)

	req := params.Entities{
		Entities: []params.Entity{{Tag: st.ModelTag().String()}},
	}
	_, err = endpoint.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelStatusSuite) TestModelStatus(c *gc.C) {
	otherModelOwner := s.Factory.MakeModelUser(c, nil)
	otherSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "dummytoo",
		Owner: otherModelOwner.UserTag,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	defer otherSt.Close()

	eight := uint64(8)
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:            []state.MachineJob{state.JobManageModel},
		Characteristics: &instance.HardwareCharacteristics{CpuCores: &eight},
		InstanceId:      "id-4",
	})
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits}, InstanceId: "id-5"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, nil),
	})

	otherFactory := factory.NewFactory(otherSt)
	otherFactory.MakeMachine(c, &factory.MachineParams{InstanceId: "id-8"})
	otherFactory.MakeMachine(c, &factory.MachineParams{InstanceId: "id-9"})
	otherFactory.MakeApplication(c, &factory.ApplicationParams{
		Charm: otherFactory.MakeCharm(c, nil),
	})

	controllerModelTag := s.State.ModelTag().String()
	hostedModelTag := otherSt.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerModelTag}, {Tag: hostedModelTag}},
	}
	results, err := s.controller.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)

	arch := "amd64"
	mem := uint64(64 * 1024 * 1024 * 1024)
	stdHw := &params.MachineHardware{
		Arch: &arch,
		Mem:  &mem,
	}
	c.Assert(results.Results, jc.DeepEquals, []params.ModelStatus{{
		ModelTag:           controllerModelTag,
		HostedMachineCount: 1,
		ApplicationCount:   1,
		OwnerTag:           s.Owner.String(),
		Life:               params.Alive,
		Machines: []params.ModelMachineInfo{
			{Id: "0", Hardware: &params.MachineHardware{Cores: &eight}, InstanceId: "id-4", Status: "pending", WantsVote: true},
			{Id: "1", Hardware: stdHw, InstanceId: "id-5", Status: "pending"},
		},
	}, {
		ModelTag:           hostedModelTag,
		HostedMachineCount: 2,
		ApplicationCount:   1,
		OwnerTag:           otherModelOwner.UserTag.String(),
		Life:               params.Alive,
		Machines: []params.ModelMachineInfo{
			{Id: "0", Hardware: stdHw, InstanceId: "id-8", Status: "pending"},
			{Id: "1", Hardware: stdHw, InstanceId: "id-9", Status: "pending"},
		},
	}})
}
