// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/uuid"
)

type dummyModelState struct {
	models map[coremodel.UUID]model.ReadOnlyModelCreationArgs
	setID  coremodel.UUID

	modelState      model.ModelState
	modelStatusInfo model.StatusInfo
}

func (d *dummyModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	if d.setID != coremodel.UUID("") {
		return modelerrors.AlreadyExists
	}
	d.models[args.UUID] = args
	d.setID = args.UUID
	return nil
}

func (d *dummyModelState) GetModel(ctx context.Context, id coremodel.UUID) (coremodel.Model, error) {
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

func (d *dummyModelState) Model(ctx context.Context) (coremodel.ReadOnlyModel, error) {
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

func (d *dummyModelState) Delete(ctx context.Context, modelUUID coremodel.UUID) error {
	delete(d.models, modelUUID)
	return nil
}

func (d *dummyModelState) GetModelState(context.Context, coremodel.UUID) (model.ModelState, error) {
	return d.modelState, nil
}

func (d *dummyModelState) GetStatus(context.Context) (model.StatusInfo, error) {
	return d.modelStatusInfo, nil
}

func (d *dummyModelState) SetStatus(_ context.Context, arg model.SetStatusArg) error {
	d.modelStatusInfo = model.StatusInfo{
		Status:  arg.Status,
		Message: arg.Message,
	}
	return nil
}

type modelServiceSuite struct {
	testing.IsolationSuite

	state          *dummyModelState
	controllerUUID uuid.UUID
}

var _ = gc.Suite(&modelServiceSuite{})

func (s *modelServiceSuite) SetUpTest(c *gc.C) {
	s.state = &dummyModelState{
		models: map[coremodel.UUID]model.ReadOnlyModelCreationArgs{},
	}

	s.controllerUUID = uuid.MustNewUUID()
}

func (s *modelServiceSuite) TestModelCreation(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.models[id] = model.ReadOnlyModelCreationArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	err := svc.CreateModel(context.Background(), s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	readonlyVal, err := svc.GetModelInfo(context.Background())
	c.Check(err, jc.ErrorIsNil)
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

func (s *modelServiceSuite) TestModelDeletion(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.models[id] = model.ReadOnlyModelCreationArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudType:   "ec2",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	err := svc.CreateModel(context.Background(), s.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.DeleteModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *modelServiceSuite) TestValidModelStatus(c *gc.C) {
	for _, v := range []corestatus.Status{
		corestatus.Available,
		corestatus.Busy,
		corestatus.Error,
	} {
		c.Assert(validSettableModelStatus(v), jc.IsTrue, gc.Commentf("status %q is not valid for a model", v))
	}
}

func (s *modelServiceSuite) TestInvalidModelStatus(c *gc.C) {
	for _, v := range []corestatus.Status{
		corestatus.Active,
		corestatus.Allocating,
		corestatus.Applied,
		corestatus.Attached,
		corestatus.Attaching,
		corestatus.Blocked,
		corestatus.Broken,
		corestatus.Detached,
		corestatus.Detaching,
		corestatus.Destroying,
		corestatus.Down,
		corestatus.Empty,
		corestatus.Executing,
		corestatus.Failed,
		corestatus.Idle,
		corestatus.Joined,
		corestatus.Joining,
		corestatus.Lost,
		corestatus.Maintenance,
		corestatus.Pending,
		corestatus.Provisioning,
		corestatus.ProvisioningError,
		corestatus.Rebooting,
		corestatus.Running,
		corestatus.Suspending,
		corestatus.Started,
		corestatus.Stopped,
		corestatus.Terminated,
		corestatus.Unknown,
		corestatus.Waiting,
		corestatus.Suspended,
	} {
		c.Assert(validSettableModelStatus(v), jc.IsFalse, gc.Commentf("status %q is valid for a model", v))
	}
}

func (s *modelServiceSuite) TestStatusDestroying(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.modelState.Destroying = true

	status, err := svc.Status(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, model.StatusInfo{
		Status:  corestatus.Destroying,
		Message: "the model is being destroyed",
	})
}

func (s *modelServiceSuite) TestStatusSuspended(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.modelState.InvalidCloudCredentialReason = "invalid credential"

	status, err := svc.Status(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, model.StatusInfo{
		Status:  corestatus.Suspended,
		Message: "suspended since cloud credential is not valid",
		Reason:  "invalid credential",
	})
}

func (s *modelServiceSuite) TestStatus(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.modelStatusInfo = model.StatusInfo{
		Status: corestatus.Available,
	}

	status, err := svc.Status(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, model.StatusInfo{
		Status: corestatus.Available,
	})
}

func (s *modelServiceSuite) TestSetStatus(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	s.state.modelStatusInfo = model.StatusInfo{
		Status: corestatus.Available,
	}

	err := svc.SetStatus(context.Background(), model.SetStatusArg{
		Status:  corestatus.Error,
		Message: "error",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.state.modelStatusInfo, gc.DeepEquals, model.StatusInfo{
		Status:  corestatus.Error,
		Message: "error",
	})
}

func (s *modelServiceSuite) TestSetStatusInvalidStatus(c *gc.C) {
	id := modeltesting.GenModelUUID(c)
	svc := NewModelService(id, s.state, s.state)

	err := svc.SetStatus(context.Background(), model.SetStatusArg{
		Status: corestatus.Suspended,
	})
	c.Assert(err, jc.ErrorIs, modelerrors.InvalidModelStatus)
}
