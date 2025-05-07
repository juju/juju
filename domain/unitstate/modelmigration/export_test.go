// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
)

type exportSuite struct {
	service *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.service = NewMockExportService(ctrl)
	return ctrl
}

func (s *exportSuite) TestExportOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	unit := app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})

	s.service.EXPECT().GetState(gomock.Any(), coreunit.Name("prometheus/0")).Return(unitstate.RetrievedUnitState{
		CharmState: map[string]string{
			"charm": "state",
		},
		UniterState: "uniter",
		RelationState: map[int]string{
			0: "relation",
		},
		StorageState: "storage",
	}, nil)

	exportOp := exportOperation{service: s.service}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unit.CharmState(), tc.DeepEquals, map[string]string{
		"charm": "state",
	})
	c.Assert(unit.UniterState(), tc.Equals, "uniter")
	c.Assert(unit.RelationState(), tc.DeepEquals, map[int]string{
		0: "relation",
	})
	c.Assert(unit.StorageState(), tc.Equals, "storage")
}

func (s *exportSuite) TestExportInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "!!!!!!!!",
	})

	exportOp := exportOperation{service: s.service}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, coreunit.InvalidUnitName)
}

func (s *exportSuite) TestExportError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})

	s.service.EXPECT().GetState(gomock.Any(), coreunit.Name("prometheus/0")).Return(unitstate.RetrievedUnitState{}, unitstateerrors.UnitNotFound)

	exportOp := exportOperation{service: s.service}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, unitstateerrors.UnitNotFound)
}
