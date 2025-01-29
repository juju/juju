// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelSuite struct {
	schematesting.ModelSuite

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()
}

func (s *modelSuite) TestCreateAndReadModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, coremodel.ModelInfo{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	})
}

func (s *modelSuite) TestDeleteModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)

	// Check that it was written correctly.
	_, err = state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithSameUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that we can't create the same model twice.

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:           id,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	err = state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithDifferentUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can only ever insert one model.

	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelAndUpdate(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:           id,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "UPDATE model SET name = 'new-name' WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable`)
}

func (s *modelSuite) TestCreateModelAndDelete(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         id,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "DELETE FROM model WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable`)
}

func (s *modelSuite) TestModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelMetrics(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().ExecContext(context.Background(), `
		INSERT INTO charm (uuid, reference_name) VALUES ('456', 'foo');
		`)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().ExecContext(context.Background(), `
		INSERT INTO application (uuid, name, life_id, charm_uuid) VALUES ('123', 'foo', 0, '456');
		`)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:            id,
			AgentVersion:    jujuversion.Current,
			ControllerUUID:  s.controllerUUID,
			Name:            "my-awesome-model",
			Type:            coremodel.IAAS,
			Cloud:           "aws",
			CloudType:       "ec2",
			CloudRegion:     "myregion",
			CredentialOwner: usertesting.GenNewName(c, "myowner"),
			CredentialName:  "mycredential",
		},
		ApplicationCount: 1,
		MachineCount:     0,
		UnitCount:        0,
	})
}

func (s *modelSuite) TestGetModelMetricsNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelCloudType(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "mymodel",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       cloudType,
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	modelCloudType, err := state.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelCloudType, jc.DeepEquals, cloudType)
}

func (s *modelSuite) TestGetModelCloudTypeNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
