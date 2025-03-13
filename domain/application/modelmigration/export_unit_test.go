// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
)

type exportUnitSuite struct {
	exportSuite

	clock *testclock.Clock
}

var _ = gc.Suite(&exportUnitSuite{})

func (s *exportUnitSuite) TestApplicationExportUnitWorkloadStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()
	s.clock = testclock.NewClock(now)

	model := description.NewModel(description.ModelArgs{})

	appArgs := description.ApplicationArgs{
		Name:     "prometheus",
		CharmURL: "ch:prometheus-1",
	}
	app := model.AddApplication(appArgs)
	app.AddUnit(description.UnitArgs{
		Name: "prometheus/0",
	})

	s.expectMinimalCharm()
	s.expectApplicationConfig()
	s.expectApplicationStatus()
	s.expectApplicationConstraints(constraints.Value{})
	s.expectGetApplicationScaleState(application.ScaleState{})

	exportOp := exportOperation{
		service: s.exportService,
		clock:   s.clock,
	}

	unitName := coreunit.Name("prometheus/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.exportService.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)

	s.exportService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), unitUUID).Return(&corestatus.StatusInfo{
		Status:  corestatus.Active,
		Since:   &now,
		Data:    map[string]interface{}{"foo": "bar"},
		Message: "it's active!",
	}, nil)
	s.exportService.EXPECT().GetUnitAgentStatus(gomock.Any(), unitUUID).Return(&corestatus.StatusInfo{
		Status:  corestatus.Idle,
		Since:   &now,
		Data:    map[string]interface{}{"foo": "baz"},
		Message: "it's idle!",
	}, nil)

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.Applications(), gc.HasLen, 1)

	app = model.Applications()[0]
	c.Check(app.Name(), gc.Equals, appArgs.Name)
	c.Check(app.CharmURL(), gc.Equals, appArgs.CharmURL)

	unit := app.Units()[0]
	c.Check(unit.Name(), gc.Equals, "prometheus/0")

	ws := unit.WorkloadStatus()
	c.Check(ws.Value(), gc.Equals, "active")
	c.Check(ws.Message(), gc.Equals, "it's active!")
	c.Check(ws.Data(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Check(ws.Updated(), gc.Equals, now)

	as := unit.AgentStatus()
	c.Check(as.Value(), gc.Equals, "idle")
	c.Check(as.Message(), gc.Equals, "it's idle!")
	c.Check(as.Data(), gc.DeepEquals, map[string]interface{}{"foo": "baz"})
	c.Check(as.Updated(), gc.Equals, now)
}
