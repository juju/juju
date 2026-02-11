// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite

	importService *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportBlank(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportMachineStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	m0 := model.AddMachine(description.MachineArgs{
		Id: "0",
	})
	m0.SetInstance(description.CloudInstanceArgs{})

	m0.SetStatus(description.StatusArgs{
		Value:   "running",
		Message: "machine is running",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})
	m0.Instance().SetStatus(description.StatusArgs{
		Value:   "active",
		Message: "instance is active",
		Data:    map[string]any{"biz": "qax"},
		Updated: now,
	})

	m1 := model.AddMachine(description.MachineArgs{
		Id: "1",
	})
	m1.SetInstance(description.CloudInstanceArgs{})

	m1.SetStatus(description.StatusArgs{
		Value:   "stopped",
		Message: "machine is stopped",
		Data:    map[string]any{"buz": "qix"},
		Updated: now,
	})
	m1.Instance().SetStatus(description.StatusArgs{
		Value:   "error",
		Message: "instance is error",
		Data:    map[string]any{"boz": "qox"},
		Updated: now,
	})

	s.importService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("0"), corestatus.StatusInfo{
		Status:  corestatus.Running,
		Message: "machine is running",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), corestatus.StatusInfo{
		Status:  corestatus.Status("active"),
		Message: "instance is active",
		Data:    map[string]any{"biz": "qax"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetMachineStatus(gomock.Any(), coremachine.Name("1"), corestatus.StatusInfo{
		Status:  corestatus.Stopped,
		Message: "machine is stopped",
		Data:    map[string]any{"buz": "qix"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("1"), corestatus.StatusInfo{
		Status:  corestatus.Error,
		Message: "instance is error",
		Data:    map[string]any{"boz": "qox"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(c.Context(), model)
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

	err := importOp.Execute(c.Context(), model)
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

	s.importService.EXPECT().SetApplicationStatus(gomock.Any(), "foo", gomock.Any()).Do(func(_ context.Context, _ string, status corestatus.StatusInfo) error {
		c.Assert(status.Status, tc.Equals, corestatus.Unset)
		c.Assert(status.Since, tc.NotNil, tc.Commentf("Since field should not be nil for NeverSet status"))
		return nil
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

	err := importOp.Execute(c.Context(), model)
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

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportRemoteApplicationOffererStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	remoteApp1 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "remote-app",
	})
	remoteApp1.SetStatus(description.StatusArgs{
		Value:   "active",
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})

	remoteApp2 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "remote-app-2",
	})
	remoteApp2.SetStatus(description.StatusArgs{
		Value:   "blocked",
		Message: "bar2",
		Data:    map[string]any{"baz2": "qux2"},
		Updated: now,
	})

	remoteApp3 := model.AddRemoteApplication(description.RemoteApplicationArgs{
		Name: "remote-123e4567e89b12d3a456426655440000",
	})
	remoteApp3.SetStatus(description.StatusArgs{
		Value:   "blocked",
		Message: "bar2",
		Data:    map[string]any{"baz2": "qux2"},
		Updated: now,
	})

	s.importService.EXPECT().SetRemoteApplicationOffererStatus(gomock.Any(), "remote-app", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "bar",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetRemoteApplicationOffererStatus(gomock.Any(), "remote-app-2", corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "bar2",
		Data:    map[string]any{"baz2": "qux2"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportFilesystemStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	fs1 := model.AddFilesystem(description.FilesystemArgs{
		ID: "fs-1",
	})
	fs1.SetStatus(description.StatusArgs{
		Value:   "active",
		Message: "active is available",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})

	fs2 := model.AddFilesystem(description.FilesystemArgs{
		ID: "fs-2",
	})
	fs2.SetStatus(description.StatusArgs{
		Value:   "error",
		Message: "error occurred",
		Data:    map[string]any{"baz2": "qux2"},
		Updated: now,
	})

	s.importService.EXPECT().SetFilesystemStatus(gomock.Any(), "fs-1", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "active is available",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetFilesystemStatus(gomock.Any(), "fs-2", corestatus.StatusInfo{
		Status:  corestatus.Error,
		Message: "error occurred",
		Data:    map[string]any{"baz2": "qux2"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumeStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	now := time.Now().UTC()

	model := description.NewModel(description.ModelArgs{})
	vol1 := model.AddVolume(description.VolumeArgs{
		ID: "vol-1",
	})
	vol1.SetStatus(description.StatusArgs{
		Value:   "active",
		Message: "volume is active",
		Data:    map[string]any{"baz": "qux"},
		Updated: now,
	})

	vol2 := model.AddVolume(description.VolumeArgs{
		ID: "vol-2",
	})
	vol2.SetStatus(description.StatusArgs{
		Value:   "pending",
		Message: "volume is pending",
		Data:    map[string]any{"baz2": "qux2"},
		Updated: now,
	})

	s.importService.EXPECT().SetVolumeStatus(gomock.Any(), "vol-1", corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "volume is active",
		Data:    map[string]any{"baz": "qux"},
		Since:   ptr(now),
	})
	s.importService.EXPECT().SetVolumeStatus(gomock.Any(), "vol-2", corestatus.StatusInfo{
		Status:  corestatus.Pending,
		Message: "volume is pending",
		Data:    map[string]any{"baz2": "qux2"},
		Since:   ptr(now),
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clock.WallClock,
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportNeverSetStatus(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	clk := testclock.NewClock(time.Now().UTC())
	now := clk.Now()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	// Don't set any status - this will be NeverSet

	_ = app.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})
	// Don't set agent or workload status - these will be NeverSet

	s.importService.EXPECT().SetApplicationStatus(gomock.Any(), "foo", corestatus.StatusInfo{
		Status: corestatus.Unset,
		Since:  &now,
	})
	s.importService.EXPECT().SetUnitAgentStatus(gomock.Any(), coreunit.Name("foo/0"), corestatus.StatusInfo{
		Status: corestatus.Unset,
		Since:  &now,
	})
	s.importService.EXPECT().SetUnitWorkloadStatus(gomock.Any(), coreunit.Name("foo/0"), corestatus.StatusInfo{
		Status: corestatus.Unset,
		Since:  &now,
	})

	importOp := importOperation{
		serviceGetter: func(u coremodel.UUID) ImportService {
			return s.importService
		},
		clock: clk,
	}

	err := importOp.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	return ctrl
}
