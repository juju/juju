// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
)

type exportSuite struct {
	testing.IsolationSuite

	exportService *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) TestExportEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)

	model := description.NewModel(description.ModelArgs{})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *exportSuite) TestExportApplicationStatuses(c *gc.C) {
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

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name: "prometheus",
	}
	app := model.AddApplication(appArgs)

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(app.Status().Message(), gc.Equals, "it's active")
	c.Check(app.Status().Value(), gc.Equals, corestatus.Active.String())
	c.Check(app.Status().Data(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *exportSuite) TestExportApplicationStatusesMissing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.exportService.EXPECT().ExportApplicationStatuses(gomock.Any()).Return(
		map[string]corestatus.StatusInfo{}, nil,
	)
	s.exportService.EXPECT().ExportUnitStatuses(gomock.Any()).Return(
		map[coreunit.Name]corestatus.StatusInfo{},
		map[coreunit.Name]corestatus.StatusInfo{},
		nil,
	)

	model := description.NewModel(description.ModelArgs{})
	appArgs := description.ApplicationArgs{
		Name: "prometheus",
	}
	app := model.AddApplication(appArgs)

	exportOp := exportOperation{
		service: s.exportService,
		clock:   clock.WallClock,
	}

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(app.Status().Value(), gc.Equals, "")
	c.Check(app.Status().NeverSet(), jc.IsTrue)
}

func (s *exportSuite) TestExportUnitStatuses(c *gc.C) {
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
		service: s.exportService,
		clock:   clock.WallClock,
	}
	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(app.Status(), gc.IsNil)

	c.Check(u0.AgentStatus().Value(), gc.Equals, corestatus.Idle.String())
	c.Check(u0.AgentStatus().Data(), gc.DeepEquals, map[string]interface{}{"agent": "idle"})
	c.Check(u0.AgentStatus().Message(), gc.Equals, "it's agent idle")

	c.Check(u0.WorkloadStatus().Value(), gc.Equals, corestatus.Active.String())
	c.Check(u0.WorkloadStatus().Data(), gc.DeepEquals, map[string]interface{}{"workload": "active"})
	c.Check(u0.WorkloadStatus().Message(), gc.Equals, "it's workload active")

	c.Check(u1.AgentStatus().Value(), gc.Equals, corestatus.Executing.String())
	c.Check(u1.AgentStatus().Data(), gc.DeepEquals, map[string]interface{}{"agent": "executing"})
	c.Check(u1.AgentStatus().Message(), gc.Equals, "it's agent executing")

	c.Check(u1.WorkloadStatus().Value(), gc.Equals, corestatus.Waiting.String())
	c.Check(u1.WorkloadStatus().Data(), gc.DeepEquals, map[string]interface{}{"workload": "waiting"})
	c.Check(u1.WorkloadStatus().Message(), gc.Equals, "it's workload waiting")
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}
