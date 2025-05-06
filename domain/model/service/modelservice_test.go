// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelagent"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/uuid"
)

type modelServiceSuite struct {
	testing.IsolationSuite

	mockControllerState        *MockControllerState
	mockModelState             *MockModelState
	mockEnvironVersionProvider *MockEnvironVersionProvider
}

func (s *modelServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockModelState = NewMockModelState(ctrl)
	s.mockEnvironVersionProvider = NewMockEnvironVersionProvider(ctrl)
	return ctrl
}

var _ = gc.Suite(&modelServiceSuite{})

func ptr[T any](v T) *T {
	return &v
}

// environVersionProviderGetter provides a test implementation of
// [EnvironVersionProviderFunc] that uses the mocked [EnvironVersionProvider] on
// this suite.
func (s *modelServiceSuite) environVersionProviderGetter() EnvironVersionProviderFunc {
	return func(string) (EnvironVersionProvider, error) {
		return s.mockEnvironVersionProvider, nil
	}
}

// TestGetModelConstraints is asserting the happy path of retrieving the set
// model constraints.
func (s *modelServiceSuite) TestGetModelConstraints(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelConstraints := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	c.Check(result, gc.DeepEquals, cons)
}

// TestGetModelConstraintsNotFound is asserting that when the state layer
// reports that no model constraints exist with an error of
// [modelerrors.ConstraintsNotFound] that we correctly handle this error and
// receive a zero value constraints object back with no error.
func (s *modelServiceSuite) TestGetModelConstraintsNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(
		constraints.Constraints{},
		modelerrors.ConstraintsNotFound,
	)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, coreconstraints.Value{})
}

// TestGetModelConstraintsFailedModelNotFound is asserting that if we ask for
// model constraints and the model does not exist in the database we get back
// an error satisfying [modelerrors.NotFound].
func (s *modelServiceSuite) TestGetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.NotFound)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestSetModelConstraints(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
			{SpaceName: "space2", Exclude: true},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return nil
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces:    ptr([]string{"space1", "^space2"}),
	}
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetModelConstraintsInvalidContainerType is asserting that if we provide
// a constraints that uses an invalid container type we get back an error that
// satisfies [machineerrors.InvalidContainerType].
func (s *modelServiceSuite) TestSetModelConstraintsInvalidContainerType(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	badConstraints := coreconstraints.Value{
		Container: ptr(instance.ContainerType("bad")),
	}
	modelCons := constraints.Constraints{
		Container: ptr(instance.ContainerType("bad")),
	}

	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return machineerrors.InvalidContainerType
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), badConstraints)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedSpaceNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces:    ptr([]string{"space1"}),
	}
	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return networkerrors.SpaceNotFound
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return modelerrors.NotFound
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetModelMetrics(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	metrics := coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:           modelUUID,
			ControllerUUID: controllerUUID,
			Name:           "my-awesome-model",
			Cloud:          "aws",
			CloudType:      "ec2",
			CloudRegion:    "myregion",
			Type:           coremodel.IAAS,
		},
	}
	s.mockModelState.EXPECT().GetModelMetrics(gomock.Any()).Return(metrics, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, metrics)
}

// TestCreateModelAgentVersionUnsupportedGreater is asserting that if we try and
// create a model with an agent version that is greater then that of the
// controller the operation fails with a [modelerrors.AgentVersionNotSupported]
// error.
func (s *modelServiceSuite) TestCreateModelAgentVersionUnsupportedGreater(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModelSeedInformation(
		gomock.Any(), modelUUID).Return(coremodel.ModelInfo{}, nil)

	agentVersion, err := semversion.Parse("99.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	err = svc.CreateModelWithAgentVersion(
		context.Background(), agentVersion,
	)
	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)
}

// TestAgentVersionUnsupportedLess is asserting that if we try and create a
// model with an agent version that is less then that of the controller.
func (s *modelServiceSuite) TestAgentVersionUnsupportedLess(c *gc.C) {
	c.Skip("This tests needs to be rewritten once tools metadata is implemented for the controller")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModelSeedInformation(
		gomock.Any(), modelUUID,
	).Return(coremodel.ModelInfo{}, nil)

	agentVersion, err := semversion.Parse("1.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err = svc.CreateModelWithAgentVersion(
		context.Background(), agentVersion,
	)
	// Add the correct error detail when restoring this test.
	c.Assert(err, gc.NotNil)
}

func (s *modelServiceSuite) TestDeleteModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().Delete(gomock.Any(), modelUUID).Return(nil)

	err := svc.DeleteModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelServiceSuite) TestDeleteModelFailedNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().Delete(gomock.Any(), modelUUID).Return(modelerrors.NotFound)

	err := svc.DeleteModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestStatusSuspended(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	svc.clock = testclock.NewClock(time.Time{})
	now := svc.clock.Now()

	s.mockControllerState.EXPECT().GetModelState(gomock.Any(), modelUUID).Return(model.ModelState{
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "invalid credential",
	}, nil)

	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Suspended)
	c.Check(status.Message, gc.Equals, "suspended since cloud credential is not valid")
	c.Check(status.Reason, gc.Equals, "invalid credential")
	c.Check(status.Since, jc.Almost, now)
}

func (s *modelServiceSuite) TestStatusDestroying(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	svc.clock = testclock.NewClock(time.Time{})
	now := svc.clock.Now()

	s.mockControllerState.EXPECT().GetModelState(gomock.Any(), modelUUID).Return(model.ModelState{
		Destroying: true,
	}, nil)

	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Destroying)
	c.Check(status.Message, gc.Equals, "the model is being destroyed")
	c.Check(status.Since, jc.Almost, now)
}

func (s *modelServiceSuite) TestStatusBusy(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	svc.clock = testclock.NewClock(time.Time{})
	now := svc.clock.Now()

	s.mockControllerState.EXPECT().GetModelState(gomock.Any(), modelUUID).Return(model.ModelState{
		Migrating: true,
	}, nil)

	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Busy)
	c.Check(status.Message, gc.Equals, "the model is being migrated")
	c.Check(status.Since, jc.Almost, now)
}

func (s *modelServiceSuite) TestStatus(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	svc.clock = testclock.NewClock(time.Time{})
	now := svc.clock.Now()

	s.mockControllerState.EXPECT().GetModelState(gomock.Any(), modelUUID).Return(model.ModelState{}, nil)

	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Available)
	c.Check(status.Since, jc.Almost, now)
}

func (s *modelServiceSuite) TestStatusFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	svc.clock = testclock.NewClock(time.Time{})

	s.mockControllerState.EXPECT().GetModelState(gomock.Any(), modelUUID).Return(model.ModelState{}, modelerrors.NotFound)

	_, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetEnvironVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("ec2", nil)
	s.mockEnvironVersionProvider.EXPECT().Version().Return(2)

	version, err := svc.GetEnvironVersion(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(version, gc.Equals, 2)
}

func (s *modelServiceSuite) TestGetEnvironVersionFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := svc.GetEnvironVersion(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetModelCloudType(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("ec2", nil)

	cloudType, err := svc.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloudType, gc.Equals, "ec2")
}

func (s *modelServiceSuite) TestGetModelCloudTypeFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := svc.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

type providerModelServiceSuite struct {
	modelServiceSuite
	mockProvider          *MockModelResourcesProvider
	mockCloudInfoProvider *MockCloudInfoProvider
}

var _ = gc.Suite(&providerModelServiceSuite{})

func (s *providerModelServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.modelServiceSuite.setupMocks(c)
	s.mockProvider = NewMockModelResourcesProvider(ctrl)
	s.mockCloudInfoProvider = NewMockCloudInfoProvider(ctrl)
	return ctrl
}

func (s *providerModelServiceSuite) TestCreateModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModelSeedInformation(gomock.Any(), gomock.Any()).Return(coremodel.ModelInfo{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentStream:    modelagent.AgentStreamReleased,
		AgentVersion:   jujuversion.Current,
	}).Return(nil)

	s.mockModelState.EXPECT().GetControllerUUID(gomock.Any()).Return(controllerUUID, nil)
	s.mockProvider.EXPECT().ValidateProviderForNewModel(gomock.Any()).Return(nil)
	s.mockProvider.EXPECT().CreateModelResources(gomock.Any(), environs.CreateParams{ControllerUUID: controllerUUID.String()}).Return(nil)

	svc := NewProviderModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		func(context.Context) (ModelResourcesProvider, error) { return s.mockProvider, nil },
		func(context.Context) (CloudInfoProvider, error) { return s.mockCloudInfoProvider, nil },
		DefaultAgentBinaryFinder(),
	)
	err := svc.CreateModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerModelServiceSuite) TestCreateModelFailedErrorAlreadyExists(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModelSeedInformation(gomock.Any(), gomock.Any()).Return(coremodel.ModelInfo{
		UUID:           modelUUID,
		Name:           "my-awesome-model",
		ControllerUUID: controllerUUID,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentStream:    modelagent.AgentStreamReleased,
		AgentVersion:   jujuversion.Current,
	}).Return(modelerrors.AlreadyExists)

	svc := NewProviderModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		func(context.Context) (ModelResourcesProvider, error) { return s.mockProvider, nil },
		func(context.Context) (CloudInfoProvider, error) { return s.mockCloudInfoProvider, nil },
		DefaultAgentBinaryFinder(),
	)
	err := svc.CreateModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *providerModelServiceSuite) TestCloudAPIVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockCloudInfoProvider.EXPECT().APIVersion().Return("666", nil)

	svc := NewProviderModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		func(context.Context) (ModelResourcesProvider, error) { return s.mockProvider, nil },
		func(context.Context) (CloudInfoProvider, error) { return s.mockCloudInfoProvider, nil },
		DefaultAgentBinaryFinder(),
	)
	vers, err := svc.CloudAPIVersion(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vers, gc.Equals, "666")
}

func (s *modelServiceSuite) TestIsControllerModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	isControllerModel, err := svc.IsControllerModel(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(isControllerModel, jc.IsTrue)

	modelUUID = modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)

	svc = NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	isControllerModel, err = svc.IsControllerModel(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(isControllerModel, jc.IsFalse)
}

func (s *modelServiceSuite) TestIsControllerModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, modelerrors.NotFound)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.IsControllerModel(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}
