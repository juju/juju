// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreconstraints "github.com/juju/juju/core/constraints"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
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

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is greater then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error.
func (s *modelServiceSuite) TestAgentVersionUnsupportedGreater(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModel(gomock.Any(), mUUID).Return(coremodel.Model{}, nil)

	agentVersion, err := version.Parse("99.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewModelService(
		mUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	err = svc.CreateModelForVersion(context.Background(), uuid.MustNewUUID(), agentVersion)
	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)
}

// TestAgentVersionUnsupportedLess is asserting that if we try and create a
// model with an agent version that is less then that of the controller.
func (s *modelServiceSuite) TestAgentVersionUnsupportedLess(c *gc.C) {
	c.Skip("This tests needs to be rewritten once tools metadata is implemented for the controller")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModel(gomock.Any(), mUUID).Return(coremodel.Model{}, nil)

	agentVersion, err := version.Parse("1.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewModelService(
		mUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	err = svc.CreateModelForVersion(context.Background(), uuid.MustNewUUID(), agentVersion)
	// Add the correct error detail when restoring this test.
	c.Assert(err, gc.NotNil)
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
	svc := NewModelService(id, s.controllerState, s.modelState, nil, DefaultAgentBinaryFinder())

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

	err := svc.CreateModel(context.Background(), s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	mInfo, err := svc.GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(mInfo, gc.Equals, coremodel.ModelInfo{
		UUID:           id,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
		AgentVersion:   jujuversion.Current,
	})
}

func (s *legacyModelServiceSuite) TestGetModelMetrics(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil, DefaultAgentBinaryFinder())

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

	err := svc.CreateModel(context.Background(), s.controllerUUID)
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
	svc := NewModelService(id, s.controllerState, s.modelState, nil, DefaultAgentBinaryFinder())

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

	err := svc.CreateModel(context.Background(), s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.DeleteModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.modelState.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *legacyModelServiceSuite) TestStatusSuspended(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState, nil, DefaultAgentBinaryFinder())
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
	svc := NewModelService(id, s.controllerState, s.modelState, nil, DefaultAgentBinaryFinder())

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

func (d *dummyModelState) GetModelConstraints(context.Context) (constraints.Constraints, error) {
	return constraints.Constraints{}, nil
}

func (d *dummyModelState) SetModelConstraints(_ context.Context, _ constraints.Constraints) error {
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
		AgentVersion:    args.AgentVersion,
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
