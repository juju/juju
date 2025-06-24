// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	statushistory "github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/testhelpers"
)

type migrationServiceSuite struct {
	testhelpers.IsolationSuite

	state         *MockMigrationState
	statusHistory *MockStatusHistory

	service *MigrationService
}

func TestMigrationServiceSuite(t *testing.T) {
	tc.Run(t, &migrationServiceSuite{})
}

func (s *migrationServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockMigrationState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)

	s.service = NewMigrationService(
		s.state,
		s.statusHistory,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.state = nil
		s.statusHistory = nil

		s.service = nil
	})

	return ctrl
}

func (s *migrationServiceSuite) TestGetMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachinesForExport(gomock.Any()).Return([]machine.ExportMachine{{
		Name: "test-machine",
		UUID: "1234",
	}}, nil)

	machines, err := s.service.GetMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.DeepEquals, []machine.ExportMachine{{
		Name: "test-machine",
		UUID: "1234",
	}})
}

func (s *migrationServiceSuite) TestGetInstanceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetInstanceID(gomock.Any(), coremachine.UUID("abc")).Return("efg", nil)

	instanceID, err := s.service.GetInstanceID(c.Context(), coremachine.UUID("abc"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID, tc.DeepEquals, instance.Id("efg"))
}

func (s *migrationServiceSuite) TestGetHardwareCharacteristics(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hwc := &instance.HardwareCharacteristics{
		Mem: ptr[uint64](1024),
	}

	s.state.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("abc")).Return(hwc, nil)

	result, err := s.service.GetHardwareCharacteristics(c.Context(), coremachine.UUID("abc"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, hwc)
}

func (s *migrationServiceSuite) TestCreateMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(nil)

	s.expectCreateMachineStatusHistory(c)

	_, err := s.service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationServiceSuite) TestCreateMachineSuccessNonce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("666"), gomock.Any(), gomock.Any(), ptr("foo")).Return(nil)

	s.expectCreateMachineStatusHistory(c)

	_, err := s.service.CreateMachine(c.Context(), "666", ptr("foo"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *migrationServiceSuite) TestCreateMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(rErr)

	_, err := s.service.CreateMachine(c.Context(), "666", nil)
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `creating machine "666": boom`)
}

// TestCreateMachineAlreadyExists asserts that the state layer returns a
// MachineAlreadyExists Error if a machine is already found with the given
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *migrationServiceSuite) TestCreateMachineAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("666"), gomock.Any(), gomock.Any(), nil).Return(machineerrors.MachineAlreadyExists)

	_, err := s.service.CreateMachine(c.Context(), coremachine.Name("666"), nil)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

func (s *migrationServiceSuite) expectCreateMachineStatusHistory(c *tc.C) {
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineNamespace.WithID("666"), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
	s.statusHistory.EXPECT().RecordStatus(gomock.Any(), domainstatus.MachineInstanceNamespace.WithID("666"), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n statushistory.Namespace, si status.StatusInfo) error {
			c.Check(si.Status, tc.Equals, status.Pending)
			return nil
		})
}
