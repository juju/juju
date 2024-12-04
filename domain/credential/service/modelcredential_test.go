// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	usertesting "github.com/juju/juju/core/user/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	jujutesting "github.com/juju/juju/internal/testing"
)

var _ = gc.Suite(&CheckMachinesSuite{})

type CheckMachinesSuite struct {
	testing.IsolationSuite

	provider *mockProvider
	instance *mockInstance

	context CredentialValidationContext

	machineService *MockMachineService
	machineState   *MockMachineState
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
		allInstancesFunc: func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
			return []instances.Instance{s.instance}, nil
		},
	}
	s.context = CredentialValidationContext{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelType:      "iaas",
	}
}

func (s *CheckMachinesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.machineState = NewMockMachineState(ctrl)
	s.context.MachineState = s.machineState
	return ctrl
}

func (s *CheckMachinesSuite) TestCheckMachinesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesInstancesMissing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "birds")
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef-1").Return("birds", nil)

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], gc.ErrorMatches, `couldn't find instance "birds" for machine 2`)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstances(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.IsNil)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstancesWhenMigrating(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}
	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, true)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], gc.ErrorMatches, `no machine with instance "analyse"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return(nil, errors.New("boom"))

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingInstances(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("", errors.New("kaboom"))

	s.provider.allInstancesFunc = func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
		return nil, errors.New("kaboom")
	}

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "kaboom")
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.container = true
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManual(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return true, nil }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManualFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return false, errors.New("manual retrieval failure") }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, gc.ErrorMatches, "manual retrieval failure")
	c.Assert(results, gc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef-1").Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0], gc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("", errors.New("retrieval failure"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef-1").Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0], gc.ErrorMatches, "getting instance id for machine 1: retrieval failure")
	c.Assert(results[1], gc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatalWhenMigrating(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("", errors.New("retrieval failure"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef-1").Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, true)
	c.Assert(err, jc.ErrorIsNil)
	// There should be 3 errors here:
	// * 2 of them because failing to get an instance id from one machine should not stop the processing the rest of the machines;
	// * 1 because we no longer can link test instance (s.instance) to a test machine (s.machine).
	c.Assert(results[0], gc.ErrorMatches, "getting instance id for machine 1: retrieval failure")
	c.Assert(results[1], gc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
	c.Assert(results[2], gc.ErrorMatches, `no machine with instance "wind-up"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesNotProvisionedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.Errorf("machine 2 %w", coreerrors.NotProvisioned) }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef").Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "deadbeef-1").Return("", machineerrors.NotProvisioned)

	// We should ignore the unprovisioned machine - we wouldn't expect
	// the cloud to know about it.
	results, err := checkMachineInstances(context.Background(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

var _ = gc.Suite(&ModelCredentialSuite{})

type ModelCredentialSuite struct {
	testing.IsolationSuite

	machineService *MockMachineService
	machineState   *MockMachineState
	context        CredentialValidationContext
}

func (s *ModelCredentialSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.context = CredentialValidationContext{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		Config:         nil,
		ModelType:      "iaas",
		Cloud:          testCloud,
		Region:         "nine",
	}
}

func (s *ModelCredentialSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.machineState = NewMockMachineState(ctrl)
	s.context.MachineState = s.machineState
	return ctrl
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialUnknownModelType(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.context.ModelType = "unknown"

	v := NewCredentialValidator()
	results, err := v.Validate(
		context.Background(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential,
		false,
	)
	c.Assert(err, gc.ErrorMatches, `model type "unknown" not supported`)
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestOpeningProviderFails(c *gc.C) {
	s.PatchValue(&newEnv, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return nil, errors.New("explosive")
	})
	results, err := checkIAASModelCredential(context.Background(), s.machineState, s.machineService, environs.OpenParams{}, false)
	c.Assert(err, gc.ErrorMatches, "explosive")
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForIAASModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{}, nil)
	s.ensureEnvForIAASModel()
	v := NewCredentialValidator()
	results, err := v.Validate(
		context.Background(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateModelCredentialCloudMismatch(c *gc.C) {
	s.ensureEnvForIAASModel()
	v := NewCredentialValidator()
	_, err := v.Validate(
		context.Background(),
		s.context,
		corecredential.Key{
			Cloud: "other",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, gc.ErrorMatches, `credential "other/bob/default" not valid`)
}

func (s *ModelCredentialSuite) TestOpeningCAASBrokerFails(c *gc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams) (caas.Broker, error) {
		return nil, errors.New("explosive")
	})
	results, err := checkCAASModelCredential(context.Background(), environs.OpenParams{})
	c.Assert(err, gc.ErrorMatches, "explosive")
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckFailed(c *gc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return nil, errors.New("fail auth") },
		}, nil
	})
	results, err := checkCAASModelCredential(context.Background(), environs.OpenParams{})
	c.Assert(err, gc.ErrorMatches, "fail auth")
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckSucceeds(c *gc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
	results, err := checkCAASModelCredential(context.Background(), environs.OpenParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForCAASModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.context.ModelType = "caas"
	s.ensureEnvForCAASModel()
	v := NewCredentialValidator()
	results, err := v.Validate(
		context.Background(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *ModelCredentialSuite) ensureEnvForCAASModel() {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
}

func (s *ModelCredentialSuite) ensureEnvForIAASModel() {
	s.PatchValue(&newEnv, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &mockEnviron{
			mockProvider: &mockProvider{
				Stub: &testing.Stub{},
				allInstancesFunc: func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
					return []instances.Instance{}, nil
				},
			},
		}, nil
	})
}

type mockProvider struct {
	*testing.Stub
	allInstancesFunc func(ctx envcontext.ProviderCallContext) ([]instances.Instance, error)
}

func (m *mockProvider) AllInstances(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
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

func (m *mockEnviron) AllInstances(ctx envcontext.ProviderCallContext) ([]instances.Instance, error) {
	return m.mockProvider.AllInstances(ctx)
}

type mockCaasBroker struct {
	caas.Broker

	namespacesFunc func() ([]string, error)
}

func (m *mockCaasBroker) Namespaces() ([]string, error) {
	return m.namespacesFunc()
}

func (m *mockCaasBroker) CheckCloudCredentials(_ context.Context) error {
	// The k8s provider implements this via a list namespaces call to the cluster
	_, err := m.namespacesFunc()
	return err
}
