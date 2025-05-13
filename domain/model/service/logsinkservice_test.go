// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type dummyLogSinkState struct {
	model *coremodel.ModelInfo
}

func (d *dummyLogSinkState) GetModelSeedInformation(ctx context.Context, modelUUID coremodel.UUID) (coremodel.ModelInfo, error) {
	if d.model != nil && d.model.UUID == modelUUID {
		return *d.model, nil
	}
	return coremodel.ModelInfo{}, modelerrors.NotFound
}

type logSinkServiceSuite struct {
	testhelpers.IsolationSuite

	state *dummyLogSinkState
}

var _ = tc.Suite(&logSinkServiceSuite{})

func (s *logSinkServiceSuite) SetUpTest(c *tc.C) {
	s.state = &dummyLogSinkState{}
}

func (s *logSinkServiceSuite) TestModel(c *tc.C) {
	svc := NewLogSinkService(s.state)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.ModelInfo{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	s.state.model = &model

	got, err := svc.Model(context.Background(), id)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(got, tc.Equals, model)
}
