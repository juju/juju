// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
)

type dummyModelState struct {
	models map[coremodel.UUID]coremodel.Model
}

func (d *dummyModelState) Create(ctx context.Context, model coremodel.Model) error {
	d.models[model.UUID] = model
	return nil
}

type modelServiceSuite struct {
	testing.IsolationSuite

	state *dummyModelState
}

var _ = gc.Suite(&modelServiceSuite{})

func (s *modelServiceSuite) SetUpTest(c *gc.C) {
	s.state = &dummyModelState{
		models: map[coremodel.UUID]coremodel.Model{},
	}
}

func (s *modelServiceSuite) TestModelCreation(c *gc.C) {
	svc := NewModelService(s.state)

	id := modeltesting.GenModelUUID(c)
	args := coremodel.Model{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		ModelType:   coremodel.IAAS,
	}
	err := svc.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	got, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)
	c.Check(got, gc.Equals, args)
}
