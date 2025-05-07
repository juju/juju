// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
)

type importSuite struct {
	testing.IsolationSuite

	importService *MockImportService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) TestImportBlank(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportApplicationStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	app.SetStatus(description.StatusArgs{
		Value:   "foo",
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})

	s.importService.EXPECT().SetApplicationStatus(gomock.Any(), "foo", corestatus.StatusInfo{
		Status:  corestatus.Status("foo"),
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	u0 := app.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})
	u0.SetAgentStatus(description.StatusArgs{
		Value:   "idle",
		Message: "agent is idle",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})
	u0.SetWorkloadStatus(description.StatusArgs{
		Value:   "active",
		Message: "unit is active",
		Data:    map[string]any{"biz": "qax"},
		Updated: now,
	})

	u1 := app.AddUnit(description.UnitArgs{
		Name: "foo/1",
	})
	u1.SetAgentStatus(description.StatusArgs{
		Value:   "executing",
		Message: "agent is executing",
		Data:    map[string]any{"buz": "qix"},
		Updated: now,
	})
	u1.SetWorkloadStatus(description.StatusArgs{
		Value:   "blocked",
		Message: "unit is blocked",
		Data:    map[string]any{"boz": "qox"},
		Updated: now,
	})

	s.importService.EXPECT().SetApplicationStatus(gomock.Any(), "foo", corestatus.StatusInfo{
		Status: corestatus.Unset,
	})
	s.importService.EXPECT().SetUnitAgentStatus(gomock.Any(), coreunit.Name("foo/0"), corestatus.StatusInfo{
		Status:  corestatus.Status("idle"),
		Message: "agent is idle",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("foo/0"), corestatus.StatusInfo{
		Status:  corestatus.Status("active"),
		Message: "unit is active",
		Data:    map[string]any{"biz": "qax"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetUnitAgentStatus(gomock.Any(), coreunit.Name("foo/1"), corestatus.StatusInfo{
		Status:  corestatus.Status("executing"),
		Message: "agent is executing",
		Data:    map[string]any{"buz": "qix"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("foo/1"), corestatus.StatusInfo{
		Status:  corestatus.Status("blocked"),
		Message: "unit is blocked",
		Data:    map[string]any{"boz": "qox"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRelationStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	clock := clock.WallClock
	now := clock.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	rel1 := model.AddRelation(description.RelationArgs{
		Id: 1,
	})
	rel2 := model.AddRelation(description.RelationArgs{
		Id: 2,
	})
	rel1.SetStatus(description.StatusArgs{
		Value:   "foo",
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})
	rel2.SetStatus(description.StatusArgs{
		Value:   "foo2",
		Message: "bar2",
		Data:    map[string]any{"baz2": "qux2"},
		Updated: now,
	})

	s.importService.EXPECT().ImportRelationStatus(gomock.Any(), 1, corestatus.StatusInfo{
		Status:  "foo",
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().ImportRelationStatus(gomock.Any(), 2, corestatus.StatusInfo{
		Status:  "foo2",
		Message: "bar2",
		Data:    map[string]any{"baz2": "qux2"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock,
	}

	err := importOp.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	return ctrl
}
