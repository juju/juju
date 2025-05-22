// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	usertesting "github.com/juju/juju/core/user/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

func TestCheckMachinesSuite(t *testing.T) {
	tc.Run(t, &CheckMachinesSuite{})
}

type CheckMachinesSuite struct {
	testhelpers.IsolationSuite

	provider *mockProvider
	instance *mockInstance

	context CredentialValidationContext

	machineService *MockMachineService
	machineState   *MockMachineState
	machine        *mockMachine
}

func (s *CheckMachinesSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	// This is what the test gets from the state.
	s.machine = createTestMachine("1", "wind-up")

	// This is what the test gets from the cloud.
	s.instance = &mockInstance{id: "wind-up"}
	s.provider = &mockProvider{
		Stub: &testhelpers.Stub{},
		allInstancesFunc: func(ctx context.Context) ([]instances.Instance, error) {
			return []instances.Instance{s.instance}, nil
		},
	}
	s.context = CredentialValidationContext{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelType:      "iaas",
	}
}

func (s *CheckMachinesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.machineState = NewMockMachineState(ctrl)
	s.context.MachineState = s.machineState
	return ctrl
}

func (s *CheckMachinesSuite) TestCheckMachinesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesInstancesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "birds")
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-1")).Return("birds", nil)

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, `couldn't find instance "birds" for machine 2`)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx context.Context) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.IsNil)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstancesWhenMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	instance2 := &mockInstance{id: "analyse"}
	s.provider.allInstancesFunc = func(ctx context.Context) ([]instances.Instance, error) {
		return []instances.Instance{s.instance, instance2}, nil
	}
	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, true)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, `no machine with instance "analyse"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return(nil, errors.New("boom"))

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("", errors.New("kaboom"))

	s.provider.allInstancesFunc = func(ctx context.Context) ([]instances.Instance, error) {
		return nil, errors.New("kaboom")
	}

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorMatches, "kaboom")
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesContainers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.container = true
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManual(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return true, nil }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesHandlesManualFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.manualFunc = func() (bool, error) { return false, errors.New("manual retrieval failure") }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorMatches, "manual retrieval failure")
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceId(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-1")).Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("", errors.New("retrieval failure"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-1")).Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 2)
	c.Check(results[0], tc.ErrorMatches, "getting instance id for machine 1: retrieval failure")
	c.Check(results[1], tc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstanceIdNonFatalWhenMigrating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.New("retrieval failure") }
	s.machine.instanceIdFunc = machine1.instanceIdFunc
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("", errors.New("retrieval failure"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-1")).Return("", errors.New("retrieval failure"))

	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, true)
	c.Assert(err, tc.ErrorIsNil)
	// There should be 3 errors here:
	// * 2 of them because failing to get an instance id from one machine should not stop the processing the rest of the machines;
	// * 1 because we no longer can link test instance (s.instance) to a test machine (s.machine).
	c.Check(results[0], tc.ErrorMatches, "getting instance id for machine 1: retrieval failure")
	c.Check(results[1], tc.ErrorMatches, "getting instance id for machine 2: retrieval failure")
	c.Check(results[2], tc.ErrorMatches, `no machine with instance "wind-up"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesNotProvisionedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machine1 := createTestMachine("2", "")
	machine1.instanceIdFunc = func() (instance.Id, error) { return "", errors.Errorf("machine 2 %w", coreerrors.NotProvisioned) }
	s.machineState.EXPECT().AllMachines().Return([]Machine{s.machine, machine1}, nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(s.machine.Id())).Return("deadbeef", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef")).Return("wind-up", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(machine1.Id())).Return("deadbeef-1", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-1")).Return("", machineerrors.NotProvisioned)

	// We should ignore the unprovisioned machine - we wouldn't expect
	// the cloud to know about it.
	results, err := checkMachineInstances(c.Context(), s.machineState, s.machineService, s.provider, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}
func TestModelCredentialSuite(t *testing.T) {
	tc.Run(t, &ModelCredentialSuite{})
}

type ModelCredentialSuite struct {
	testhelpers.IsolationSuite

	machineService *MockMachineService
	machineState   *MockMachineState
	context        CredentialValidationContext
}

func (s *ModelCredentialSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.context = CredentialValidationContext{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		Config:         nil,
		ModelType:      "iaas",
		Cloud:          testCloud,
		Region:         "nine",
	}
}

func (s *ModelCredentialSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.machineState = NewMockMachineState(ctrl)
	s.context.MachineState = s.machineState
	return ctrl
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialUnknownModelType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.context.ModelType = "unknown"

	v := NewCredentialValidator()
	results, err := v.Validate(
		c.Context(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential,
		false,
	)
	c.Assert(err, tc.ErrorMatches, `model type "unknown" not supported`)
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestOpeningProviderFails(c *tc.C) {
	s.PatchValue(&newEnv, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.Environ, error) {
		return nil, errors.New("explosive")
	})
	results, err := checkIAASModelCredential(c.Context(), s.machineState, s.machineService, environs.OpenParams{}, false)
	c.Assert(err, tc.ErrorMatches, "explosive")
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForIAASModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineState.EXPECT().AllMachines().Return([]Machine{}, nil)
	s.ensureEnvForIAASModel()
	v := NewCredentialValidator()
	results, err := v.Validate(
		c.Context(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateModelCredentialCloudMismatch(c *tc.C) {
	s.ensureEnvForIAASModel()
	v := NewCredentialValidator()
	_, err := v.Validate(
		c.Context(),
		s.context,
		corecredential.Key{
			Cloud: "other",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, tc.ErrorMatches, `credential "other/bob/default" not valid`)
}

func (s *ModelCredentialSuite) TestOpeningCAASBrokerFails(c *tc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error) {
		return nil, errors.New("explosive")
	})
	results, err := checkCAASModelCredential(c.Context(), environs.OpenParams{})
	c.Assert(err, tc.ErrorMatches, "explosive")
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckFailed(c *tc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return nil, errors.New("fail auth") },
		}, nil
	})
	results, err := checkCAASModelCredential(c.Context(), environs.OpenParams{})
	c.Assert(err, tc.ErrorMatches, "fail auth")
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestCAASCredentialCheckSucceeds(c *tc.C) {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
	results, err := checkCAASModelCredential(c.Context(), environs.OpenParams{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) TestValidateNewModelCredentialForCAASModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.context.ModelType = "caas"
	s.ensureEnvForCAASModel()
	v := NewCredentialValidator()
	results, err := v.Validate(
		c.Context(),
		s.context,
		corecredential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "default",
		},
		&testCredential, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *ModelCredentialSuite) ensureEnvForCAASModel() {
	s.PatchValue(&newCAASBroker, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (caas.Broker, error) {
		return &mockCaasBroker{
			namespacesFunc: func() ([]string, error) { return []string{}, nil },
		}, nil
	})
}

func (s *ModelCredentialSuite) ensureEnvForIAASModel() {
	s.PatchValue(&newEnv, func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.Environ, error) {
		return &mockEnviron{
			mockProvider: &mockProvider{
				Stub: &testhelpers.Stub{},
				allInstancesFunc: func(ctx context.Context) ([]instances.Instance, error) {
					return []instances.Instance{}, nil
				},
			},
		}, nil
	})
}

type mockProvider struct {
	*testhelpers.Stub
	allInstancesFunc func(ctx context.Context) ([]instances.Instance, error)
}

func (m *mockProvider) AllInstances(ctx context.Context) ([]instances.Instance, error) {
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

func (m *mockEnviron) AllInstances(ctx context.Context) ([]instances.Instance, error) {
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
