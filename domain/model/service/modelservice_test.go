// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/uuid"
)

type modelServiceSuite struct {
	testing.IsolationSuite

	controllerState *dummyControllerModelState
	modelState      *dummyModelState
	controllerUUID  uuid.UUID
}

var _ = gc.Suite(&modelServiceSuite{})

func (s *modelServiceSuite) SetUpTest(c *gc.C) {
	s.controllerState = &dummyControllerModelState{
		models:     map[coremodel.UUID]model.ReadOnlyModelCreationArgs{},
		modelState: map[coremodel.UUID]model.ModelState{},
	}
	s.modelState = &dummyModelState{
		models: map[coremodel.UUID]model.ReadOnlyModelCreationArgs{},
	}

	s.controllerUUID = uuid.MustNewUUID()
}

func (s *modelServiceSuite) TestModelConstraints(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	cons := constraints.MustParse("mem=1G")
	s.modelState.constraints = cons

	readCons, err := svc.ModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(readCons, gc.DeepEquals, cons)
}

func (s *modelServiceSuite) TestSetModelConstraints(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	cons := constraints.MustParse("mem=1G")
	err := svc.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.modelState.constraints, gc.DeepEquals, cons)
}

func (s *modelServiceSuite) TestModelCreation(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	m := model.ReadOnlyModelCreationArgs{
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

	readonlyVal, err := svc.GetModelInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(readonlyVal, gc.Equals, coremodel.ReadOnlyModel{
		UUID:           id,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	})
}

func (s *modelServiceSuite) TestGetModelMetrics(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	m := model.ReadOnlyModelCreationArgs{
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
		Model: coremodel.ReadOnlyModel{
			UUID:           id,
			ControllerUUID: s.controllerUUID,
			Name:           "my-awesome-model",
			Cloud:          "aws",
			CloudType:      "ec2",
			CloudRegion:    "myregion",
			Type:           coremodel.IAAS,
		}})
}

func (s *modelServiceSuite) TestModelDeletion(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	m := model.ReadOnlyModelCreationArgs{
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

func (s *modelServiceSuite) TestStatusSuspended(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)
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

func (s *modelServiceSuite) TestStatusDestroying(c *gc.C) {
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

func (s *modelServiceSuite) TestStatusBusy(c *gc.C) {
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

func (s *modelServiceSuite) TestStatus(c *gc.C) {
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

func (s *modelServiceSuite) TestStatusFailedModelNotFound(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.controllerState, s.modelState)

	_, err := svc.GetStatus(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

type dummyControllerModelState struct {
	models     map[coremodel.UUID]model.ReadOnlyModelCreationArgs
	modelState map[coremodel.UUID]model.ModelState
}

func (d *dummyControllerModelState) GetModel(ctx context.Context, id coremodel.UUID) (coremodel.Model, error) {
	args, exists := d.models[id]
	if !exists {
		return coremodel.Model{}, modelerrors.NotFound
	}

	return coremodel.Model{
		UUID:         args.UUID,
		Name:         args.Name,
		ModelType:    args.Type,
		AgentVersion: args.AgentVersion,
		Cloud:        args.Cloud,
		CloudType:    args.CloudType,
		CloudRegion:  args.CloudRegion,
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
	models      map[coremodel.UUID]model.ReadOnlyModelCreationArgs
	setID       coremodel.UUID
	constraints constraints.Value
}

func (d *dummyModelState) ModelConstraints(context.Context) (constraints.Value, error) {
	return d.constraints, nil
}

func (d *dummyModelState) SetModelConstraints(_ context.Context, cons constraints.Value) error {
	d.constraints = cons
	return nil
}

func (d *dummyModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	if d.setID != coremodel.UUID("") {
		return modelerrors.AlreadyExists
	}
	d.models[args.UUID] = args
	d.setID = args.UUID
	return nil
}

func (d *dummyModelState) GetModel(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	if d.setID == coremodel.UUID("") {
		return coremodel.ReadOnlyModel{}, modelerrors.NotFound
	}

	args := d.models[d.setID]
	return coremodel.ReadOnlyModel{
		UUID:            args.UUID,
		AgentVersion:    args.AgentVersion,
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
		Model: coremodel.ReadOnlyModel{
			UUID:            args.UUID,
			AgentVersion:    args.AgentVersion,
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

func (d *dummyModelState) Delete(ctx context.Context, modelUUID coremodel.UUID) error {
	delete(d.models, modelUUID)
	return nil
}
