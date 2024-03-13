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
	modelerrors "github.com/juju/juju/domain/model/errors"
)

type dummyProviderState struct {
	model *coremodel.ReadOnlyModel
}

func (d *dummyProviderState) Model(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	if d.model != nil {
		return *d.model, nil
	}
	return coremodel.ReadOnlyModel{}, modelerrors.NotFound
}

type providerServiceSuite struct {
	testing.IsolationSuite

	state *dummyProviderState
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) SetUpTest(c *gc.C) {
	s.state = &dummyProviderState{}
}

func (s *providerServiceSuite) TestModel(c *gc.C) {
	svc := NewProviderService(s.state)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.ReadOnlyModel{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	s.state.model = &model

	got, err := svc.Model(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(got, gc.Equals, model)
}
