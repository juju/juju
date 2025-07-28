// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type dummyObjectStoreState struct {
	model *coremodel.ModelInfo
}

func (d *dummyObjectStoreState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	if d.model != nil {
		return *d.model, nil
	}
	return coremodel.ModelInfo{}, modelerrors.NotFound
}

type objectStoreServiceSuite struct {
	testhelpers.IsolationSuite

	state *dummyObjectStoreState

	mockControllerState *MockState
	mockWatcherFactory  *MockWatcherFactory
}

func TestObjectStoreServiceSuite(t *testing.T) {
	tc.Run(t, &objectStoreServiceSuite{})
}

func (s *objectStoreServiceSuite) SetUpTest(c *tc.C) {
	s.state = &dummyObjectStoreState{}
}

func (s *objectStoreServiceSuite) TestModel(c *tc.C) {
	svc := NewObjectStoreService(s.state, nil)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.ModelInfo{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	s.state.model = &model

	got, err := svc.Model(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(got, tc.Equals, model)
}
