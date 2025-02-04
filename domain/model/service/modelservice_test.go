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

	"github.com/juju/juju/core/constraints"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/version"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/uuid"
)

type modelServiceSuite struct {
	testing.IsolationSuite

	mockControllerState        *MockControllerState
	mockEnvironVersionProvider *MockEnvironVersionProvider
	mockModelState             *MockModelState
}

func (s *modelServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockEnvironVersionProvider = NewMockEnvironVersionProvider(ctrl)
	s.mockModelState = NewMockModelState(ctrl)
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
	return func(_ string) (EnvironVersionProvider, error) {
		return s.mockEnvironVersionProvider, nil
	}
}

// TestGetModelConstraints is asserting the happy path of retrieving the set
// model constraints.
func (s *modelServiceSuite) TestGetModelConstraints(c *gc.C) {
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

	svc := NewModelService(
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
func (s *modelServiceSuite) TestGetModelConstraintsNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(
		model.Constraints{},
		modelerrors.ConstraintsNotFound,
	)

	svc := NewModelService(
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
func (s *modelServiceSuite) TestGetModelConstraintsFailedModelNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(model.Constraints{}, modelerrors.NotFound)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	_, err := svc.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestSetModelConstraints(c *gc.C) {
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

	svc := NewModelService(
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
func (s *modelServiceSuite) TestSetModelConstraintsInvalidContainerType(c *gc.C) {
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

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), badConstraints)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedSpaceNotFound(c *gc.C) {
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

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedModelNotFound(c *gc.C) {
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

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
	)
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

type legacyModelServiceSuite struct {
	testing.IsolationSuite

	controllerState *dummyControllerModelState
	modelState      *dummyModelState
	controllerUUID  uuid.UUID
}

var _ = gc.Suite(&legacyModelServiceSuite{})

func (s *legacyModelServiceSuite) SetUpTest(c *gc.C) {
	s.controllerState = &dummyControllerModelState{
		models:     map[coremodel.UUID]model.ModelDetailArgs{},
		modelState: map[coremodel.UUID]model.ModelState{},
	}
	s.modelState = &dummyModelState{
		models: map[coremodel.UUID]model.ModelDetailArgs{},
	}

	s.controllerUUID = uuid.MustNewUUID()
}

func (s *legacyModelServiceSuite) TestModelCreation(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil)

	m := model.ModelDetailArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}

	s.controllerState.models[id] = m
	s.modelState.models[id] = m

	err := svc.CreateModel(context.Background(), s.controllerUUID, version.Current)
	c.Assert(err, jc.ErrorIsNil)

	readonlyVal, err := svc.GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(readonlyVal, gc.Equals, coremodel.ModelInfo{
		UUID:           id,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	})
}

func (s *legacyModelServiceSuite) TestGetModelMetrics(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil)

	m := model.ModelDetailArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}

	s.controllerState.models[id] = m
	s.modelState.models[id] = m

	err := svc.CreateModel(context.Background(), s.controllerUUID, version.Current)
	c.Assert(err, jc.ErrorIsNil)

	readonlyVal, err := svc.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(readonlyVal, gc.Equals, coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:           id,
			ControllerUUID: s.controllerUUID,
			Name:           "my-awesome-model",
			Cloud:          "aws",
			CloudType:      "ec2",
			CloudRegion:    "myregion",
			Type:           coremodel.IAAS,
		}})
}

func (s *legacyModelServiceSuite) TestModelDeletion(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil)

	m := model.ModelDetailArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}

	s.controllerState.models[id] = m
	s.modelState.models[id] = m

	err := svc.CreateModel(context.Background(), s.controllerUUID, version.Current)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.DeleteModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.modelState.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *legacyModelServiceSuite) TestStatusSuspended(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil)
	svc.clock = testclock.NewClock(time.Time{})

	s.modelState.setID = id
	s.controllerState.modelState[id] = model.ModelState{
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "invalid credential",
	}

	now := svc.clock.Now()
	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Suspended)
	c.Check(status.Message, gc.Equals, "suspended since cloud credential is not valid")
	c.Check(status.Reason, gc.Equals, "invalid credential")
	c.Check(status.Since, jc.Almost, now)
}

func (s *legacyModelServiceSuite) TestStatusDestroying(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := &ModelService{
		clock:        testclock.NewClock(time.Time{}),
		modelID:      id,
		controllerSt: s.controllerState,
		modelSt:      s.modelState,
	}

	s.modelState.setID = id
	s.controllerState.modelState[id] = model.ModelState{
		Destroying: true,
	}

	now := svc.clock.Now()
	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Destroying)
	c.Check(status.Message, gc.Equals, "the model is being destroyed")
	c.Check(status.Since, jc.Almost, now)
}

func (s *legacyModelServiceSuite) TestStatusBusy(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := &ModelService{
		clock:        testclock.NewClock(time.Time{}),
		modelID:      id,
		controllerSt: s.controllerState,
		modelSt:      s.modelState,
	}

	s.modelState.setID = id
	s.controllerState.modelState[id] = model.ModelState{
		Migrating: true,
	}

	now := svc.clock.Now()
	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Busy)
	c.Check(status.Message, gc.Equals, "the model is being migrated")
	c.Check(status.Since, jc.Almost, now)
}

func (s *legacyModelServiceSuite) TestStatus(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := &ModelService{
		clock:        testclock.NewClock(time.Time{}),
		modelID:      id,
		controllerSt: s.controllerState,
		modelSt:      s.modelState,
	}

	s.modelState.setID = id
	s.controllerState.modelState[id] = model.ModelState{}

	now := svc.clock.Now()
	status, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, corestatus.Available)
	c.Check(status.Since, jc.Almost, now)
}

func (s *legacyModelServiceSuite) TestStatusFailedModelNotFound(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil)

	_, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

type dummyControllerModelState struct {
	models     map[coremodel.UUID]model.ModelDetailArgs
	modelState map[coremodel.UUID]model.ModelState
}

func (d *dummyControllerModelState) GetModel(ctx context.Context, id coremodel.UUID) (coremodel.Model, error) {
	args, exists := d.models[id]
	if !exists {
		return coremodel.Model{}, modelerrors.NotFound
	}

	return coremodel.Model{
		UUID:        args.UUID,
		Name:        args.Name,
		ModelType:   args.Type,
		Cloud:       args.Cloud,
		CloudType:   args.CloudType,
		CloudRegion: args.CloudRegion,
		Credential: corecredential.Key{
			Name:  args.CredentialName,
			Owner: args.CredentialOwner,
			Cloud: args.Cloud,
		},
		OwnerName: args.CredentialOwner,
	}, nil
}

func (d *dummyControllerModelState) GetModelState(_ context.Context, modelUUID coremodel.UUID) (model.ModelState, error) {
	mState, ok := d.modelState[modelUUID]
	if !ok {
		return model.ModelState{}, modelerrors.NotFound
	}
	return mState, nil
}

type dummyModelState struct {
	models map[coremodel.UUID]model.ModelDetailArgs
	setID  coremodel.UUID
}

func (d *dummyModelState) GetModelConstraints(context.Context) (model.Constraints, error) {
	return model.Constraints{}, nil
}

func (d *dummyModelState) SetModelConstraints(_ context.Context, _ model.Constraints) error {
	return nil
}

func (d *dummyModelState) Create(ctx context.Context, args model.ModelDetailArgs) error {
	if d.setID != coremodel.UUID("") {
		return modelerrors.AlreadyExists
	}
	d.models[args.UUID] = args
	d.setID = args.UUID
	return nil
}

func (d *dummyModelState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	if d.setID == coremodel.UUID("") {
		return coremodel.ModelInfo{}, modelerrors.NotFound
	}

	args := d.models[d.setID]
	return coremodel.ModelInfo{
		UUID:            args.UUID,
		ControllerUUID:  args.ControllerUUID,
		Name:            args.Name,
		Type:            args.Type,
		Cloud:           args.Cloud,
		CloudType:       args.CloudType,
		CloudRegion:     args.CloudRegion,
		CredentialOwner: args.CredentialOwner,
		CredentialName:  args.CredentialName,
	}, nil
}

func (d *dummyModelState) GetModelMetrics(ctx context.Context) (coremodel.ModelMetrics, error) {
	if d.setID == coremodel.UUID("") {
		return coremodel.ModelMetrics{}, modelerrors.NotFound
	}

	args := d.models[d.setID]
	return coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:            args.UUID,
			ControllerUUID:  args.ControllerUUID,
			Name:            args.Name,
			Type:            args.Type,
			Cloud:           args.Cloud,
			CloudType:       args.CloudType,
			CloudRegion:     args.CloudRegion,
			CredentialOwner: args.CredentialOwner,
			CredentialName:  args.CredentialName,
		},
	}, nil
}

func (d *dummyModelState) GetModelCloudType(ctx context.Context) (string, error) {
	if d.setID == coremodel.UUID("") {
		return "", modelerrors.NotFound
	}

	args := d.models[d.setID]

	return args.CloudType, nil
}

func (d *dummyModelState) Delete(ctx context.Context, modelUUID coremodel.UUID) error {
	delete(d.models, modelUUID)
	return nil
}
