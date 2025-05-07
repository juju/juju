// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
)

type importSuite struct {
	service *MockImportService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockImportService(ctrl)
	return ctrl
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
		CharmState: map[string]string{
			"charm": "state",
		},
		UniterState: "uniter",
		RelationState: map[int]string{
			0: "relation",
		},
		StorageState: "storage",
	})

	s.service.EXPECT().SetState(gomock.Any(), unitstate.UnitState{
		Name: coreunit.Name("prometheus/0"),
		CharmState: &map[string]string{
			"charm": "state",
		},
		UniterState: ptr("uniter"),
		RelationState: &map[int]string{
			0: "relation",
		},
		StorageState: ptr("storage"),
	})

	importOp := importOperation{service: s.service}
	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportPartial(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name:         "prometheus/0",
		UniterState:  "uniter",
		StorageState: "storage",
	})

	s.service.EXPECT().SetState(gomock.Any(), unitstate.UnitState{
		Name:         coreunit.Name("prometheus/0"),
		UniterState:  ptr("uniter"),
		StorageState: ptr("storage"),
	})

	importOp := importOperation{service: s.service}
	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name:         "prometheus/0",
		UniterState:  "uniter",
		StorageState: "storage",
	})

	s.service.EXPECT().SetState(gomock.Any(), unitstate.UnitState{
		Name:         coreunit.Name("prometheus/0"),
		UniterState:  ptr("uniter"),
		StorageState: ptr("storage"),
	}).Return(unitstateerrors.UnitNotFound)

	importOp := importOperation{service: s.service}
	err := importOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, unitstateerrors.UnitNotFound)
}

func ptr[T any](v T) *T {
	return &v
}
