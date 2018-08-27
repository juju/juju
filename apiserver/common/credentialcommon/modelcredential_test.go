// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
)

type ModelCredentialSuite struct {
	testing.IsolationSuite

	provider           *mockProvider
	instance           *mockInstance
	callContext        context.ProviderCallContext
	testNewEnvironFunc credentialcommon.NewEnvironFunc

	backend *mockBackend
	machine *mockMachine
}

var _ = gc.Suite(&ModelCredentialSuite{})

func (s *ModelCredentialSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// This is what the test gets from the state.
	s.machine = createTestMachine("1", "wind-up")
	s.backend = &mockBackend{
		Stub: &testing.Stub{},
		allMachinesFunc: func() ([]credentialcommon.Machine, error) {
			return []credentialcommon.Machine{s.machine}, nil
		},
	}

	// This is what the test gets from the cloud.
	s.instance = &mockInstance{id: "wind-up"}
	s.provider = &mockProvider{
		Stub: &testing.Stub{},
		allInstancesFunc: func(ctx context.ProviderCallContext) ([]instance.Instance, error) {
			return []instance.Instance{s.instance}, nil
		},
	}

	s.testNewEnvironFunc = func(args environs.OpenParams) (environs.Environ, error) {
		return &mockEnviron{mockProvider: s.provider}, nil
	}
	s.callContext = context.NewCloudCallContext()
}

func (s *ModelCredentialSuite) TestCheckMachinesSuccess(c *gc.C) {
	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCheckMachinesInstancesMissing(c *gc.C) {
	machine1 := createTestMachine("2", "birds")
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return []credentialcommon.Machine{s.machine, machine1}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `couldn't find instance "birds" for machine 2`)
}

func (s *ModelCredentialSuite) TestCheckMachinesExtraInstances(c *gc.C) {
	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx context.ProviderCallContext) ([]instance.Instance, error) {
		return []instance.Instance{s.instance, instance2}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `no machine with instance "analyse"`)
}

func (s *ModelCredentialSuite) TestCheckMachinesErrorGettingMachines(c *gc.C) {
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return nil, errors.New("boom")
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCheckMachinesErrorGettingInstances(c *gc.C) {
	s.provider.allInstancesFunc = func(ctx context.ProviderCallContext) ([]instance.Instance, error) {
		return nil, errors.New("kaboom")
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, gc.ErrorMatches, "kaboom")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCheckMachinesHandlesContainers(c *gc.C) {
	machine1 := createTestMachine("1", "")
	machine1.container = true
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return []credentialcommon.Machine{s.machine, machine1}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCheckMachinesHandlesManual(c *gc.C) {
	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return false, errors.New("manual retrieval failure") }
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return []credentialcommon.Machine{s.machine, machine1}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, gc.ErrorMatches, "manual retrieval failure")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})

	machine1.manualFunc = func() (bool, error) { return true, nil }
	results, err = credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCheckMachinesErrorGettingMachineInstanceId(c *gc.C) {
	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return []credentialcommon.Machine{s.machine, machine1}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: common.ServerError(errors.Errorf("getting instance id for machine 2: retrieval failure"))},
		},
	})
}

func (s *ModelCredentialSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatal(c *gc.C) {
	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.backend.allMachinesFunc = func() ([]credentialcommon.Machine, error) {
		return []credentialcommon.Machine{s.machine, machine1}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	// There should be 3 errors here:
	// * 2 of them because failing to get an instance id from one machine should not stop the processing the rest of the machines;
	// * 1 because we no longer can link test instance (s.instance) to a test machine (s.machine).
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: common.ServerError(errors.New("getting instance id for machine 1: retrieval failure"))},
			{Error: common.ServerError(errors.New("getting instance id for machine 2: retrieval failure"))},
			{Error: common.ServerError(errors.New(`no machine with instance "wind-up"`))},
		},
	})
}

func (s *ModelCredentialSuite) TestValidateModelCredential(c *gc.C) {
	results, err := credentialcommon.ValidateModelCredential(s.backend, s.provider, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialErrorGettingModel(c *gc.C) {
	s.backend.SetErrors(errors.New("get model error"))
	results, err := credentialcommon.ValidateNewModelCredential(s.createModelBackend(c), s.testNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, nil)
	c.Assert(err, gc.ErrorMatches, "get model error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialErrorGettingCloud(c *gc.C) {
	s.backend.SetErrors(
		nil, // getting model
		errors.New("get cloud error"),
	)
	results, err := credentialcommon.ValidateNewModelCredential(s.createModelBackend(c), s.testNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, nil)
	c.Assert(err, gc.ErrorMatches, "get cloud error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialErrorGettingModelConfig(c *gc.C) {
	model := createTestModel()
	model.configFunc = func() (*config.Config, error) {
		return nil, errors.New("get model config error")
	}

	backend := s.createModelBackend(c)
	backend.modelFunc = func() (credentialcommon.Model, error) {
		return model, nil
	}

	results, err := credentialcommon.ValidateNewModelCredential(backend, s.testNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, &testCredential)
	c.Assert(err, gc.ErrorMatches, "get model config error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialErrorValidateCredentialForModelCloud(c *gc.C) {
	model := createTestModel()
	model.validateCredentialFunc = func(tag names.CloudCredentialTag, credential cloud.Credential) error {
		return errors.New("credential not for model cloud error")
	}

	backend := s.createModelBackend(c)
	backend.modelFunc = func() (credentialcommon.Model, error) {
		return model, nil
	}

	results, err := credentialcommon.ValidateNewModelCredential(backend, s.testNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, &testCredential)
	c.Assert(err, gc.ErrorMatches, "credential not for model cloud error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialErrorOpenEnviron(c *gc.C) {
	failNewEnvironFunc := func(args environs.OpenParams) (environs.Environ, error) {
		return nil, errors.New("new environ error")
	}
	results, err := credentialcommon.ValidateNewModelCredential(s.createModelBackend(c), failNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, &testCredential)
	c.Assert(err, gc.ErrorMatches, "new environ error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialSuccess(c *gc.C) {
	results, err := credentialcommon.ValidateNewModelCredential(s.createModelBackend(c), s.testNewEnvironFunc, s.callContext, names.CloudCredentialTag{}, &testCredential)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) createModelBackend(c *gc.C) *mockModelBackend {
	return &mockModelBackend{
		mockBackend: s.backend,
		modelFunc: func() (credentialcommon.Model, error) {
			return createTestModel(), s.backend.NextErr()
		},
		cloudFunc: func(name string) (cloud.Cloud, error) {
			return cloud.Cloud{
				Name:      "nuage",
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType, cloud.UserPassAuthType},
				Regions:   []cloud.Region{{Name: "nine", Endpoint: "endpoint"}},
			}, s.backend.NextErr()
		},
	}
}

type mockBackend struct {
	*testing.Stub
	allMachinesFunc func() ([]credentialcommon.Machine, error)
}

func (m *mockBackend) AllMachines() ([]credentialcommon.Machine, error) {
	m.MethodCall(m, "AllMachines")
	return m.allMachinesFunc()
}

type mockProvider struct {
	*testing.Stub
	allInstancesFunc func(ctx context.ProviderCallContext) ([]instance.Instance, error)
}

func (m *mockProvider) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	m.MethodCall(m, "AllInstances", ctx)
	return m.allInstancesFunc(ctx)
}

type mockInstance struct {
	instance.Instance
	id string
}

func (i *mockInstance) Id() instance.Id {
	return instance.Id(i.id)
}

type mockMachine struct {
	id             string
	container      bool
	manualFunc     func() (bool, error)
	instanceIdFunc func() (instance.Id, error)
}

func (m *mockMachine) IsManual() (bool, error) {
	return m.manualFunc()
}

func (m *mockMachine) IsContainer() bool {
	return m.container
}

func (m *mockMachine) InstanceId() (instance.Id, error) {
	return m.instanceIdFunc()
}

func (m *mockMachine) Id() string {
	return m.id
}

func createTestMachine(id, instanceId string) *mockMachine {
	return &mockMachine{
		id:             id,
		manualFunc:     func() (bool, error) { return false, nil },
		instanceIdFunc: func() (instance.Id, error) { return instance.Id(instanceId), nil },
	}
}

type mockModelBackend struct {
	*mockBackend

	modelFunc func() (credentialcommon.Model, error)
	cloudFunc func(name string) (cloud.Cloud, error)
}

func (m *mockModelBackend) Model() (credentialcommon.Model, error) {
	m.MethodCall(m, "Model")
	return m.modelFunc()
}

func (m *mockModelBackend) Cloud(name string) (cloud.Cloud, error) {
	m.MethodCall(m, "Cloud", name)
	return m.cloudFunc(name)
}

type mockModel struct {
	cloudFunc              func() string
	cloudRegionFunc        func() string
	configFunc             func() (*config.Config, error)
	validateCredentialFunc func(tag names.CloudCredentialTag, credential cloud.Credential) error
}

func (m *mockModel) Cloud() string {
	return m.cloudFunc()
}

func (m *mockModel) CloudRegion() string {
	return m.cloudRegionFunc()
}

func (m *mockModel) Config() (*config.Config, error) {
	return m.configFunc()
}

func (m *mockModel) ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	return m.validateCredentialFunc(tag, credential)
}

func createTestModel() *mockModel {
	return &mockModel{
		cloudFunc:       func() string { return "nuage" },
		cloudRegionFunc: func() string { return "nine" },
		configFunc: func() (*config.Config, error) {
			return nil, nil
		},
		validateCredentialFunc: func(tag names.CloudCredentialTag, credential cloud.Credential) error {
			return nil
		},
	}
}

var (
	testCredential = cloud.NewCredential(
		cloud.UserPassAuthType,
		map[string]string{
			"username": "user",
			"password": "password",
		},
	)
)

type mockEnviron struct {
	environs.Environ
	*mockProvider
}

func (m *mockEnviron) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	return m.mockProvider.AllInstances(ctx)
}
