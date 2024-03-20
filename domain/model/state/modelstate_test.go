// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type modelSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) TestCreateModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.Model{
		UUID:        id,
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	}
	err := state.Create(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	row := db.QueryRowContext(context.Background(), "SELECT uuid, name, type, cloud, cloud_region FROM model WHERE uuid = $1", id)

	var got coremodel.Model
	err = row.Scan(&got.UUID, &got.Name, &got.ModelType, &got.Cloud, &got.CloudRegion)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(got, jc.DeepEquals, model)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithSameUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that we can't create the same model twice.

	id := modeltesting.GenModelUUID(c)
	args := coremodel.Model{
		UUID:        id,
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
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

	err := state.Create(context.Background(), coremodel.Model{
		UUID:        modeltesting.GenModelUUID(c),
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = state.Create(context.Background(), coremodel.Model{
		UUID:        modeltesting.GenModelUUID(c),
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelAndUpdate(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), coremodel.Model{
		UUID:        id,
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "UPDATE model SET name = 'new-name' WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is read-only`)
}

func (s *modelSuite) TestCreateModelAndDelete(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner)

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), coremodel.Model{
		UUID:        id,
		Name:        "my-awesome-model",
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "DELETE FROM model WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable`)
}
