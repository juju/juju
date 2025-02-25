// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/uuid"
)

type migrationServiceSuite struct {
	testing.IsolationSuite

	mockControllerState        *MockControllerState
	mockEnvironVersionProvider *MockEnvironVersionProvider
	mockModelState             *MockModelState
}

func (s *migrationServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockEnvironVersionProvider = NewMockEnvironVersionProvider(ctrl)
	s.mockModelState = NewMockModelState(ctrl)
	return ctrl
}

var _ = gc.Suite(&migrationServiceSuite{})

// environVersionProviderGetter provides a test implementation of
// [EnvironVersionProviderFunc] that uses the mocked [EnvironVersionProvider] on
// this suite.
func (s *migrationServiceSuite) environVersionProviderGetter() EnvironVersionProviderFunc {
	return func(_ string) (EnvironVersionProvider, error) {
		return s.mockEnvironVersionProvider, nil
	}
}

// TestGetModelConstraints is asserting the happy path of retrieving the set
// model constraints.
func (s *migrationServiceSuite) TestGetModelConstraints(c *gc.C) {
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

	svc := NewMigrationService(
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
func (s *migrationServiceSuite) TestGetModelConstraintsNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(
		constraints.Constraints{},
		modelerrors.ConstraintsNotFound,
	)

	svc := NewMigrationService(
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
func (s *migrationServiceSuite) TestGetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.NotFound)

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *migrationServiceSuite) TestSetModelConstraints(c *gc.C) {
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

	svc := NewMigrationService(
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
func (s *migrationServiceSuite) TestSetModelConstraintsInvalidContainerType(c *gc.C) {
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

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), badConstraints)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)
}

func (s *migrationServiceSuite) TestSetModelConstraintsFailedSpaceNotFound(c *gc.C) {
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

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *migrationServiceSuite) TestSetModelConstraintsFailedModelNotFound(c *gc.C) {
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

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *migrationServiceSuite) TestCreateModelForVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModel(gomock.Any(), gomock.Any()).Return(coremodel.Model{
		UUID:        modelUUID,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		ModelType:   coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentVersion:   jujuversion.Current,
	}).Return(nil)

	svc := NewMigrationService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.CreateModelForVersion(context.Background(), controllerUUID, jujuversion.Current)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestCreateModelForVersionFailedErrorAlreadyExists(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModel(gomock.Any(), gomock.Any()).Return(coremodel.Model{
		UUID:        modelUUID,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		ModelType:   coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentVersion:   jujuversion.Current,
	}).Return(modelerrors.AlreadyExists)

	svc := NewMigrationService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.CreateModelForVersion(context.Background(), controllerUUID, jujuversion.Current)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *migrationServiceSuite) TestDeleteModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewMigrationService(
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

func (s *migrationServiceSuite) TestDeleteModelFailedNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewMigrationService(
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

func (s *migrationServiceSuite) TestGetEnvironVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewMigrationService(
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

func (s *migrationServiceSuite) TestGetEnvironVersionFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewMigrationService(
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
