// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon_test

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/common/credentialcommon/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CheckMachinesSuite{})

type CheckMachinesSuite struct {
	testing.IsolationSuite

	provider    *mockProvider
	instance    *mockInstance
	callContext context.ProviderCallContext

	machineService *mocks.MockMachineService
	machine        *mockMachine
}

func (s *CheckMachinesSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	// This is what the test gets from the state.
	s.machine = createTestMachine("1", "wind-up")

	// This is what the test gets from the cloud.
	s.instance = &mockInstance{id: "wind-up"}
	s.provider = &mockProvider{
		Stub: &testing.Stub{},
		allInstancesFunc: func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
			return []instances.Instance{s.instance}, nil
		},
	}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *CheckMachinesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = mocks.NewMockMachineService(ctrl)
	return ctrl
}

func (s *CheckMachinesSuite) TestCheckMachinesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesInstancesMissing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "birds")
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `couldn't find instance "birds" for machine 2`)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstances(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine}, nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.IsNil)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstancesWhenMigrating(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine}, nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}
	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `no machine with instance "analyse"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().AllMachines().Return(nil, errors.New("boom"))

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingInstances(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine}, nil)

	s.provider.allInstancesFunc = func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
		return nil, errors.New("kaboom")
	}

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "kaboom")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.container = true
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManual(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return true, nil }
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManualFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return false, errors.New("manual retrieval failure") }
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "manual retrieval failure")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.Errorf("getting instance id for machine 2: retrieval failure"))},
		},
	})
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatal(c *gc.C) {
	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("getting instance id for machine 1: retrieval failure"))},
			{Error: apiservererrors.ServerError(errors.New("getting instance id for machine 2: retrieval failure"))},
		},
	})
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatalWhenMigrating(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, true)
	c.Assert(err, jc.ErrorIsNil)
	// There should be 3 errors here:
	// * 2 of them because failing to get an instance id from one machine should not stop the processing the rest of the machines;
	// * 1 because we no longer can link test instance (s.instance) to a test machine (s.machine).
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservererrors.ServerError(errors.New("getting instance id for machine 1: retrieval failure"))},
			{Error: apiservererrors.ServerError(errors.New("getting instance id for machine 2: retrieval failure"))},
			{Error: apiservererrors.ServerError(errors.New(`no machine with instance "wind-up"`))},
		},
	})
}

func (s *CheckMachinesSuite) TestCheckMachinesNotProvisionedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.NotProvisionedf("machine 2") }
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{s.machine, machine1}, nil)

	// We should ignore the unprovisioned machine - we wouldn't expect
	// the cloud to know about it.
	results, err := credentialcommon.CheckMachineInstances(s.callContext, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

var _ = gc.Suite(&ModelCredentialSuite{})

type ModelCredentialSuite struct {
	testing.IsolationSuite

	machineService    *mocks.MockMachineService
	credentialService *mocks.MockCredentialService
	model             *mocks.MockModel
	modelConfigErr    error

	callContext context.ProviderCallContext
}

func (s *ModelCredentialSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.callContext = context.NewEmptyCloudCallContext()
	s.modelConfigErr = nil
}

func (s *ModelCredentialSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.credentialService = mocks.NewMockCredentialService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)

	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().ControllerUUID().Return(jujuesting.ControllerTag.Id()).AnyTimes()
	s.model.EXPECT().CloudRegion().Return("nine")
	s.model.EXPECT().Config().DoAndReturn(func() (*config.Config, error) {
		return nil, s.modelConfigErr
	})
	return ctrl
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialUnknownModelType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.model.EXPECT().Type().Return(state.ModelType("unknown")).MinTimes(1)

	results, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.NewCloudCredentialTag("dummy/bob/default"),
		&testCredential,
		testCloud, false,
	)
	c.Assert(err, gc.ErrorMatches, `model type "unknown" not supported`)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestBuildingOpenParamsErrorGettingModelConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.modelConfigErr = errors.New("get model config error")

	results, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.NewCloudCredentialTag("dummy/bob/default"),
		&testCredential, testCloud, false)
	c.Assert(err, gc.ErrorMatches, "get model config error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestBuildingOpenParamsErrorValidateCredentialForModelCloud(c *gc.C) {
	c.Skip("TODO(wallyworld) - need to move to dqlite for this test")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	//model.validateCredentialFunc = func(tag names.CloudCredentialTag, credential cloud.Credential) error {
	//	return errors.New("credential not for model cloud error")
	//}

	results, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.CloudCredentialTag{},
		&testCredential, testCloud, false)
	c.Assert(err, gc.ErrorMatches, "credential not for model cloud error")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateExistingModelCredentialUnsetCloudCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.model = mocks.NewMockModel(ctrl)
	s.credentialService = mocks.NewMockCredentialService(ctrl)

	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{}, nil)
	s.ensureEnvForIAASModel()
	results, err := credentialcommon.ValidateExistingModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.CloudCredentialTag{},
		s.credentialService, testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateExistingModelCredentialErrorGettingCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.credentialService = mocks.NewMockCredentialService(ctrl)

	tag := names.NewCloudCredentialTag("cirrus/fred/default")
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), tag).Return(cloud.Credential{}, errors.New("no nope niet"))

	results, err := credentialcommon.ValidateExistingModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		tag,
		s.credentialService,
		testCloud, false)
	c.Assert(err, gc.ErrorMatches, "no nope niet")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateExistingModelCredentialInvalidCredential(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.credentialService = mocks.NewMockCredentialService(ctrl)

	tag := names.NewCloudCredentialTag("cirrus/fred/default")
	cred := cloud.NewEmptyCredential()
	cred.Label = "cred"
	cred.Invalid = true
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), tag).Return(cred, nil)

	results, err := credentialcommon.ValidateExistingModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		tag,
		s.credentialService,
		testCloud, false)
	c.Assert(err, gc.ErrorMatches, `credential "cred" not valid`)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestOpeningProviderFails(c *gc.C) {
	s.PatchValue(credentialcommon.NewEnv, func(stdcontext.Context, environs.OpenParams) (environs.Environ, error) {
		return nil, errors.New("explosive")
	})
	results, err := credentialcommon.CheckIAASModelCredential(s.callContext, environs.OpenParams{}, s.machineService, false)
	c.Assert(err, gc.ErrorMatches, "explosive")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForIAASModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.model.EXPECT().Type().Return(state.ModelTypeIAAS)
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{}, nil)
	s.ensureEnvForIAASModel()
	results, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.NewCloudCredentialTag("dummy/bob/default"),
		&testCredential, testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateModelCredentialCloudMismatch(c *gc.C) {
	s.ensureEnvForIAASModel()
	_, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.NewCloudCredentialTag("other/bob/default"),
		&testCredential, testCloud, false)
	c.Assert(err, gc.ErrorMatches, `credential "other/bob/default" not valid`)
}

func (s *ModelCredentialSuite) TestValidateExistingModelCredentialForIAASModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.model.EXPECT().Type().Return(state.ModelTypeIAAS)
	s.machineService.EXPECT().AllMachines().Return([]credentialcommon.Machine{}, nil)

	tag := names.NewCloudCredentialTag("dummy/bob/default")
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), tag).Return(cloud.Credential{}, nil)

	s.ensureEnvForIAASModel()
	results, err := credentialcommon.ValidateExistingModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		tag,
		s.credentialService, testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestOpeningCAASBrokerFails(c *gc.C) {
	s.PatchValue(credentialcommon.NewCAASBroker, func(stdcontext.Context, environs.OpenParams) (caas.Broker, error) {
		return nil, errors.New("explosive")
	})
	results, err := credentialcommon.CheckCAASModelCredential(stdcontext.Background(), environs.OpenParams{})
	c.Assert(err, gc.ErrorMatches, "explosive")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckFailed(c *gc.C) {
	s.PatchValue(credentialcommon.NewCAASBroker, func(stdcontext.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return nil, errors.New("fail auth") },
		}, nil
	})
	results, err := credentialcommon.CheckCAASModelCredential(stdcontext.Background(), environs.OpenParams{})
	c.Assert(err, gc.ErrorMatches, "fail auth")
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckSucceeds(c *gc.C) {
	s.PatchValue(credentialcommon.NewCAASBroker, func(stdcontext.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
	results, err := credentialcommon.CheckCAASModelCredential(stdcontext.Background(), environs.OpenParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForCAASModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.model.EXPECT().Type().Return(state.ModelTypeCAAS)

	s.ensureEnvForCAASModel()
	results, err := credentialcommon.ValidateNewModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		names.NewCloudCredentialTag("dummy/bob/default"),
		&testCredential, testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) TestValidateExistingModelCredentialForCAASSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.model.EXPECT().Type().Return(state.ModelTypeCAAS)

	tag := names.NewCloudCredentialTag("dummy/bob/default")
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), tag).Return(cloud.Credential{}, nil)

	s.ensureEnvForCAASModel()
	results, err := credentialcommon.ValidateExistingModelCredential(
		s.callContext,
		s.model,
		s.machineService,
		tag,
		s.credentialService, testCloud, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{})
}

func (s *ModelCredentialSuite) ensureEnvForCAASModel() {
	s.PatchValue(credentialcommon.NewCAASBroker, func(stdcontext.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
}

func (s *ModelCredentialSuite) ensureEnvForIAASModel() {
	s.PatchValue(credentialcommon.NewEnv, func(stdcontext.Context, environs.OpenParams) (environs.Environ, error) {
		return &mockEnviron{
			mockProvider: &mockProvider{
				Stub: &testing.Stub{},
				allInstancesFunc: func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
					return []instances.Instance{}, nil
				},
			},
		}, nil
	})
}

type mockProvider struct {
	*testing.Stub
	allInstancesFunc func(ctx context.ProviderCallContext) ([]instances.Instance, error)
}

func (m *mockProvider) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	m.MethodCall(m, "AllInstances", ctx)
	return m.allInstancesFunc(ctx)
}

type mockInstance struct {
	instances.Instance
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

func (m *mockMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.instanceIdFunc()
	return instId, "", err
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

var (
	testCloud = cloud.Cloud{
		Name:    "dummy",
		Regions: []cloud.Region{{Name: "nine"}},
	}
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

func (m *mockEnviron) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return m.mockProvider.AllInstances(ctx)
}

type mockCaasBroker struct {
	caas.Broker

	namespacesFunc func() ([]string, error)
}

func (m *mockCaasBroker) Namespaces() ([]string, error) {
	return m.namespacesFunc()
}

func (m *mockCaasBroker) CheckCloudCredentials() error {
	// The k8s provider implements this via a list namespaces call to the cluster
	_, err := m.namespacesFunc()
	return err
}
