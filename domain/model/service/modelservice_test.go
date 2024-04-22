// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain/model"
)

type dummyModelState struct {
	models map[coremodel.UUID]model.ReadOnlyModelCreationArgs
}

func (d *dummyModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	d.models[args.UUID] = args
	return nil
}

func (d *dummyModelState) Delete(ctx context.Context, modelUUID coremodel.UUID) error {
	delete(d.models, modelUUID)
	return nil
}

type modelServiceSuite struct {
	baseSuite

	state *dummyModelState
}

var _ = gc.Suite(&modelServiceSuite{})

func (s *modelServiceSuite) SetUpTest(c *gc.C) {
	s.state = &dummyModelState{
		models: map[coremodel.UUID]model.ReadOnlyModelCreationArgs{},
	}
}

func (s *modelServiceSuite) TestModelCreation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewModelService(s.state)

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Owner:       "admin",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	err := svc.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	got, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)
	c.Check(got, gc.Equals, args)
}

func (s *modelServiceSuite) TestModelDeletion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewModelService(s.state)

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
		UUID:        id,
		Name:        "my-awesome-model",
		Owner:       "admin",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	err := svc.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	err = svc.DeleteModel(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}
