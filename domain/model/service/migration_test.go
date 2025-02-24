// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
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

	modelConstraints := model.Constraints{
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
	)
	result, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)

	cons := constraints.Value{
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
		model.Constraints{},
		modelerrors.ConstraintsNotFound,
	)

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	result, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, constraints.Value{})
}

// TestGetModelConstraintsFailedModelNotFound is asserting that if we ask for
// model constraints and the model does not exist in the database we get back
// an error satisfying [modelerrors.NotFound].
func (s *migrationServiceSuite) TestGetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(model.Constraints{}, modelerrors.NotFound)

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	_, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *migrationServiceSuite) TestSetModelConstraints(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelCons := model.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]model.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
			{SpaceName: "space2", Exclude: true},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m model.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return nil
		})

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)

	cons := constraints.Value{
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

	badConstraints := constraints.Value{
		Container: ptr(instance.ContainerType("bad")),
	}
	modelCons := model.Constraints{
		Container: ptr(instance.ContainerType("bad")),
	}

	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m model.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return machineerrors.InvalidContainerType
		})

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), badConstraints)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)
}

func (s *migrationServiceSuite) TestSetModelConstraintsFailedSpaceNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := constraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces:    ptr([]string{"space1"}),
	}
	modelCons := model.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]model.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m model.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return networkerrors.SpaceNotFound
		})

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *migrationServiceSuite) TestSetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := constraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	modelCons := model.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m model.Constraints) error {
			c.Check(m, jc.DeepEquals, modelCons)
			return modelerrors.NotFound
		})

	svc := NewMigrationService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *migrationServiceSuite) TestCreateModel(c *gc.C) {
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
	}).Return(nil)

	svc := NewMigrationService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.CreateModel(context.Background(), controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestCreateModelFailedErrorAlreadyExists(c *gc.C) {
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
	}).Return(modelerrors.AlreadyExists)

	svc := NewMigrationService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.CreateModel(context.Background(), controllerUUID)
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
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := svc.GetEnvironVersion(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
