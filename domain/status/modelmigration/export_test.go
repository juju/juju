// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/testhelpers"
)

type exportSuite struct {
	testhelpers.IsolationSuite

	exportService *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) TestExportEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)
	s.exportService.EXPECT().ExportRelationStatuses(gomock.Any()).Return(
		map[int]corestatus.StatusInfo{}, nil,
	)

	model := description.NewModel(description.ModelArgs{})

	exportOp := exportOperation{
		serviceGetter: func(u coremodel.UUID) ExportService {
			return s.exportService
		},
		clock: clock.WallClock,
	}

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportSuite) TestExportApplicationStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{
			"prometheus": {
				Message: "it's active",
				Status:  corestatus.Active,
				Data:    map[string]interface{}{"foo": "bar"},
			},
		}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)
	s.exportService.EXPECT().ExportRelationStatuses(gomock.Any()).Return(
		map[int]corestatus.StatusInfo{}, nil,
	)

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name: "prometheus",
	}
	app := model.AddApplication(appArgs)

	exportOp := exportOperation{
		serviceGetter: func(u coremodel.UUID) ExportService {
			return s.exportService
		},
		clock: clock.WallClock,
	}

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(app.Status().Message(), tc.Equals, "it's active")
	c.Check(app.Status().Value(), tc.Equals, corestatus.Active.String())
	c.Check(app.Status().Data(), tc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *exportSuite) TestExportApplicationStatusesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)
	s.exportService.EXPECT().ExportRelationStatuses(gomock.Any()).Return(
		map[int]corestatus.StatusInfo{}, nil,
	)

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name: "prometheus",
	}
	app := model.AddApplication(appArgs)

	exportOp := exportOperation{
		serviceGetter: func(u coremodel.UUID) ExportService {
			return s.exportService
		},
		clock: clock.WallClock,
	}

	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(app.Status().Value(), tc.Equals, "")
	c.Check(app.Status().NeverSet(), tc.IsTrue)
}

func (s *exportSuite) TestExportUnitStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{
			"prometheus/0": {
				Message: "it's workload active",
				Status:  corestatus.Active,
				Data:    map[string]interface{}{"workload": "active"},
			},
			"prometheus/1": {
				Message: "it's workload waiting",
				Status:  corestatus.Waiting,
				Data:    map[string]interface{}{"workload": "waiting"},
			},
		},
		map[coreunit.Name]corestatus.StatusInfo{
			"prometheus/0": {
				Message: "it's agent idle",
				Status:  corestatus.Idle,
				Data:    map[string]interface{}{"agent": "idle"},
			},
			"prometheus/1": {
				Message: "it's agent executing",
				Status:  corestatus.Executing,
				Data:    map[string]interface{}{"agent": "executing"},
			},
		},
		nil,
	)
	s.exportService.EXPECT().ExportRelationStatuses(gomock.Any()).Return(
		map[int]corestatus.StatusInfo{}, nil,
	)

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	u0 := app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})
	u1 := app.AddUnit(description.UnitArgs{
		Name: "prometheus/1",
	})

	exportOp := exportOperation{
		serviceGetter: func(u coremodel.UUID) ExportService {
			return s.exportService
		},
		clock: clock.WallClock,
	}
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(app.Status().NeverSet(), tc.IsTrue)

	c.Check(u0.AgentStatus().Value(), tc.Equals, corestatus.Idle.String())
	c.Check(u0.AgentStatus().Data(), tc.DeepEquals, map[string]interface{}{"agent": "idle"})
	c.Check(u0.AgentStatus().Message(), tc.Equals, "it's agent idle")

	c.Check(u0.WorkloadStatus().Value(), tc.Equals, corestatus.Active.String())
	c.Check(u0.WorkloadStatus().Data(), tc.DeepEquals, map[string]interface{}{"workload": "active"})
	c.Check(u0.WorkloadStatus().Message(), tc.Equals, "it's workload active")

	c.Check(u1.AgentStatus().Value(), tc.Equals, corestatus.Executing.String())
	c.Check(u1.AgentStatus().Data(), tc.DeepEquals, map[string]interface{}{"agent": "executing"})
	c.Check(u1.AgentStatus().Message(), tc.Equals, "it's agent executing")

	c.Check(u1.WorkloadStatus().Value(), tc.Equals, corestatus.Waiting.String())
	c.Check(u1.WorkloadStatus().Data(), tc.DeepEquals, map[string]interface{}{"workload": "waiting"})
	c.Check(u1.WorkloadStatus().Message(), tc.Equals, "it's workload waiting")
}

func (s *exportSuite) TestExportRelationStatuses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)
	s.exportService.EXPECT().ExportRelationStatuses(gomock.Any()).Return(
		map[int]corestatus.StatusInfo{
			1: {
				Status: corestatus.Joining,
			},
		},
		nil,
	)

	model := description.NewModel(description.ModelArgs{})
	rel1 := model.AddRelation(description.RelationArgs{
		Id: 1,
	})
	rel2 := model.AddRelation(description.RelationArgs{
		Id: 2,
	})

	exportOp := exportOperation{
		serviceGetter: func(u coremodel.UUID) ExportService {
			return s.exportService
		},
		clock: clock.WallClock,
	}
	err := exportOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(rel1.Status().Value(), tc.Equals, corestatus.Joining.String())
	c.Check(rel2.Status().NeverSet(), tc.IsTrue)
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}
