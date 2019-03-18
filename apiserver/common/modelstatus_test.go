// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
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
	s.NewPolicy = func(*state.State) state.Policy {
		return statePolicy{}
	}

	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}

	controller, err := controller.NewControllerAPIv7(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      s.authorizer,
			StatePool_: s.StatePool,
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
	endpoint, err := controller.NewControllerAPIv6(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
		})
	c.Assert(err, jc.ErrorIsNil)
	controllerModelTag := s.Model.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerModelTag}},
	}
	result, err := endpoint.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
}

func (s *modelStatusSuite) TestModelStatusOwnerAllowed(c *gc.C) {
	// Set up the user making the call.
	owner := s.Factory.MakeUser(c, nil)
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: owner.Tag(),
	}
	st := s.Factory.MakeModel(c, &factory.ModelParams{Owner: owner.Tag()})
	defer st.Close()
	endpoint, err := controller.NewControllerAPIv6(
		facadetest.Context{
			State_:     s.State,
			Resources_: s.resources,
			Auth_:      anAuthoriser,
			StatePool_: s.StatePool,
		})
	c.Assert(err, jc.ErrorIsNil)

	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	req := params.Entities{
		Entities: []params.Entity{{Tag: model.ModelTag().String()}},
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
		DisplayName:     "snowflake",
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped",
				Size: 123,
			},
		}},
	})
	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: "id-5",
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{
				Pool: "modelscoped",
				Size: 123,
			},
		}, {
			Filesystem: state.FilesystemParams{
				Pool: "machinescoped",
				Size: 123,
			},
		}},
	})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, nil),
	})

	otherFactory := factory.NewFactory(otherSt, s.StatePool)
	otherFactory.MakeMachine(c, &factory.MachineParams{InstanceId: "id-8"})
	otherFactory.MakeMachine(c, &factory.MachineParams{InstanceId: "id-9"})
	otherFactory.MakeApplication(c, &factory.ApplicationParams{
		Charm: otherFactory.MakeCharm(c, nil),
	})

	otherModel, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerModelTag := s.Model.ModelTag().String()
	hostedModelTag := otherModel.ModelTag().String()

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
	c.Assert(results.Results, jc.DeepEquals, []params.ModelStatus{
		{
			ModelTag:           controllerModelTag,
			HostedMachineCount: 1,
			ApplicationCount:   1,
			OwnerTag:           s.Owner.String(),
			Life:               params.Alive,
			Machines: []params.ModelMachineInfo{
				{Id: "0", Hardware: &params.MachineHardware{Cores: &eight}, InstanceId: "id-4", DisplayName: "snowflake", Status: "pending", WantsVote: true},
				{Id: "1", Hardware: stdHw, InstanceId: "id-5", Status: "pending"},
			},
			Volumes: []params.ModelVolumeInfo{{
				Id: "0", Status: "pending", Detachable: true,
			}},
			Filesystems: []params.ModelFilesystemInfo{{
				Id: "0", Status: "pending", Detachable: true,
			}, {
				Id: "1/1", Status: "pending", Detachable: false,
			}},
		},
		{
			ModelTag:           hostedModelTag,
			HostedMachineCount: 2,
			ApplicationCount:   1,
			OwnerTag:           otherModelOwner.UserTag.String(),
			Life:               params.Alive,
			Machines: []params.ModelMachineInfo{
				{Id: "0", Hardware: stdHw, InstanceId: "id-8", Status: "pending"},
				{Id: "1", Hardware: stdHw, InstanceId: "id-9", Status: "pending"},
			},
		},
	})
}

func (s *modelStatusSuite) TestModelStatusCAAS(c *gc.C) {
	otherModelOwner := s.Factory.MakeModelUser(c, nil)
	otherSt := s.Factory.MakeCAASModel(c, &factory.ModelParams{
		Owner: otherModelOwner.UserTag,
		ConfigAttrs: testing.Attrs{
			"controller": false,
		},
	})
	defer otherSt.Close()

	otherFactory := factory.NewFactory(otherSt, s.StatePool)
	otherFactory.MakeApplication(c, &factory.ApplicationParams{
		Charm: otherFactory.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"}),
	})

	otherModel, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	controllerModelTag := s.Model.ModelTag().String()
	hostedModelTag := otherModel.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerModelTag}, {Tag: hostedModelTag}},
	}
	results, err := s.controller.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, jc.DeepEquals, []params.ModelStatus{
		{
			ModelTag:           controllerModelTag,
			HostedMachineCount: 0,
			ApplicationCount:   0,
			OwnerTag:           s.Owner.String(),
			Life:               params.Alive,
		},
		{
			ModelTag:           hostedModelTag,
			HostedMachineCount: 0,
			ApplicationCount:   1,
			OwnerTag:           otherModelOwner.UserTag.String(),
			Life:               params.Alive,
		},
	})
}

func (s *modelStatusSuite) TestModelStatusRunsForAllModels(c *gc.C) {
	req := params.Entities{
		Entities: []params.Entity{
			{Tag: "fail.me"},
			{Tag: s.Model.ModelTag().String()},
		},
	}
	expected := params.ModelStatusResults{
		Results: []params.ModelStatus{
			{
				Error: common.ServerError(errors.New(`"fail.me" is not a valid tag`))},
			{
				ModelTag: s.Model.ModelTag().String(),
				Life:     params.Life(s.Model.Life().String()),
				OwnerTag: s.Model.Owner().String(),
			},
		},
	}
	result, err := s.controller.ModelStatus(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}

type statePolicy struct{}

func (statePolicy) Prechecker() (environs.InstancePrechecker, error) {
	return nil, errors.NotImplementedf("Prechecker")
}

func (statePolicy) ConfigValidator() (config.Validator, error) {
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (statePolicy) ConstraintsValidator(context.ProviderCallContext) (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (statePolicy) InstanceDistributor() (context.Distributor, error) {
	return nil, errors.NotImplementedf("InstanceDistributor")
}

func (statePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	return storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (statePolicy) ProviderConfigSchemaSource(cloudName string) (config.ConfigSchemaSource, error) {
	return nil, errors.NotImplementedf("ConfigSchemaSource")
}
