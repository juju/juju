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

	exportOp := exportOperation{
		service: s.exportService,
		clock:   s.clock,
	}

	s.exportService.EXPECT().GetUnitWorkloadStatus(gomock.Any(), coreunit.Name("prometheus/0")).Return(&corestatus.StatusInfo{
		Status:  corestatus.Active,
		Since:   &now,
		Data:    map[string]interface{}{"foo": "bar"},
		Message: "it's active!",
	}, nil)

	err := exportOp.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model.Applications(), gc.HasLen, 1)

	app = model.Applications()[0]
	c.Check(app.Name(), gc.Equals, appArgs.Name)
	c.Check(app.CharmURL(), gc.Equals, appArgs.CharmURL)

	unit := app.Units()[0]
	c.Check(unit.Name(), gc.Equals, "prometheus/0")
	c.Check(unit.WorkloadStatus().Value(), gc.Equals, "active")
	c.Check(unit.WorkloadStatus().Message(), gc.Equals, "it's active!")
	c.Check(unit.WorkloadStatus().Data(), gc.DeepEquals, map[string]interface{}{"foo": "bar"})
	c.Check(unit.WorkloadStatus().Updated(), gc.Equals, now)
}
