// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
	jujuversion "github.com/juju/juju/version"
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

func (s *modelSuite) TestCreateModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudRegion:     "myregion",
		CredentialOwner: "myowner",
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.Model(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, coremodel.ReadOnlyModel{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudRegion:     "myregion",
		CredentialOwner: "myowner",
		CredentialName:  "mycredential",
	})
}

// TestAgentVersion is testing the happy path of AgentVersion to make sure the
// correct value is being reported back.
func (s *modelSuite) TestAgentVersion(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudRegion:     "myregion",
		CredentialOwner: "myowner",
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)

	version, err := state.AgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(version, jc.DeepEquals, jujuversion.Current)
}

// TestAgentVersionNotFound is testing that when no agent version has been set
// for the model we get back an error that satisfies [errors.NotFound]
func (s *modelSuite) TestAgentVersionNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	_, err := state.AgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *modelSuite) TestDeleteModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudRegion:     "myregion",
		CredentialOwner: "myowner",
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)

	// Check that it was written correctly.
	_, err = state.Model(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithSameUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that we can't create the same model twice.

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:           id,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudRegion:    "myregion",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	err = state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithDifferentUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that you can only ever insert one model.

	err := state.Create(context.Background(), model.ReadOnlyModelCreationArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = state.Create(context.Background(), model.ReadOnlyModelCreationArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelAndUpdate(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ReadOnlyModelCreationArgs{
		UUID:           id,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudRegion:    "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "UPDATE model SET name = 'new-name' WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable`)
}

func (s *modelSuite) TestCreateModelAndDelete(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ReadOnlyModelCreationArgs{
		UUID:         id,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "DELETE FROM model WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable`)
}

func (s *modelSuite) TestModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	_, err := state.Model(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
