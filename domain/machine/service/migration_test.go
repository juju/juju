// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/machine"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationServiceSuite struct {
	testhelpers.IsolationSuite

	state *MockMigrationState
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)

	return ctrl
}

func (s *migrationServiceSuite) TestGetMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachinesForExport(gomock.Any()).Return([]machine.ExportMachine{{
		Name: "test-machine",
		UUID: "1234",
	}}, nil)

	machines, err := NewMigrationService(s.state, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{{
		Name: "test-machine",
		UUID: "1234",
	}})
}

func (s *migrationServiceSuite) TestGetInstanceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetInstanceID(gomock.Any(), coremachine.UUID("abc")).Return("efg", nil)

	instanceID, err := NewMigrationService(s.state, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceID(c.Context(), coremachine.UUID("abc"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID, tc.DeepEquals, instance.Id("efg"))
}

func (s *migrationServiceSuite) TestGetHardwareCharacteristics(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hwc := &instance.HardwareCharacteristics{
		Mem: ptr[uint64](1024),
	}

	s.state.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("abc")).Return(hwc, nil)

	result, err := NewMigrationService(s.state, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetHardwareCharacteristics(c.Context(), coremachine.UUID("abc"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, hwc)
}
