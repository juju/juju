// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state         *MockState
	statusHistory *MockStatusHistory
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)

	return ctrl
}

// TestDeleteMachineSuccess asserts the happy path of the DeleteMachine service.
func (s *serviceSuite) TestDeleteMachineSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), machine.Name("666")).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		DeleteMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
}

// TestDeleteMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestDeleteMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), machine.Name("666")).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		DeleteMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `deleting machine "666": boom`)
}

// TestGetLifeSuccess asserts the happy path of the GetMachineLife service.
func (s *serviceSuite) TestGetLifeSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	alive := life.Alive
	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(alive, nil)

	l, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, corelife.Alive)
}

// TestGetLifeError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetLifeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(-1, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `getting life status for machine "666": boom`)
}

// TestGetLifeNotFoundError asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetLifeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(-1, coreerrors.NotFound)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestSetMachineLifeSuccess asserts the happy path of the SetMachineLife
// service.
func (s *serviceSuite) TestSetMachineLifeSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Alive).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineLife(c.Context(), "666", life.Alive)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetMachineLifeError asserts that an error coming from the state layer is
// preserved, and passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineLifeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Alive).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineLife(c.Context(), "666", life.Alive)
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `setting life status for machine "666": boom`)
}

// TestSetMachineLifeMachineDoNotExist asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestSetMachineLifeMachineDoNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("nonexistent"), life.Alive).Return(coreerrors.NotFound)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineLife(c.Context(), "nonexistent", life.Alive)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestEnsureDeadMachineSuccess asserts the happy path of the EnsureDeadMachine
// service function.
func (s *serviceSuite) TestEnsureDeadMachineSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Dead).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		EnsureDeadMachine(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
}

// TestEnsureDeadMachineError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestEnsureDeadMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Dead).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		EnsureDeadMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestListAllMachinesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AllMachineNames(gomock.Any()).Return([]machine.Name{"666"}, nil)

	machines, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machines, tc.DeepEquals, []machine.Name{"666"})
}

// TestListAllMachinesError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestListAllMachinesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AllMachineNames(gomock.Any()).Return(nil, rErr)

	machines, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(machines, tc.IsNil)
}

func (s *serviceSuite) TestInstanceIdSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("123", nil)

	instanceId, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, instance.Id("123"))
}

// TestInstanceIdError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestInstanceIdError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("", rErr)

	instanceId, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(instanceId, tc.Equals, instance.UnknownId)
}

// TestInstanceIdNotProvisionedError asserts that the state layer returns a
// NotProvisioned Error if an instanceId is not found for the given machineName,
// and that error is preserved and passed on to the service layer to be handled
// there.
func (s *serviceSuite) TestInstanceIdNotProvisionedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("", machineerrors.NotProvisioned)

	instanceId, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(instanceId, tc.Equals, instance.UnknownId)
}

func (s *serviceSuite) TestGetInstanceIDByMachineNameSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return(machineUUID, nil)
	s.state.EXPECT().GetInstanceID(gomock.Any(), machineUUID).Return("i-foo", nil)

	instanceId, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceIDByMachineName(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, instance.Id("i-foo"))
}

func (s *serviceSuite) TestGetInstanceIDByMachineNameNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return("", machineerrors.MachineNotFound)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceIDByMachineName(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *serviceSuite) TestGetInstanceIDByMachineNameNotProvisioned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)
	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return(machineUUID, nil)
	s.state.EXPECT().GetInstanceID(gomock.Any(), machineUUID).Return("", machineerrors.NotProvisioned)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetInstanceIDByMachineName(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

// TestIsMachineControllerSuccess asserts the happy path of the
// IsMachineController service.
func (s *serviceSuite) TestIsMachineControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(true, nil)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsTrue)
}

// TestIsMachineControllerError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsMachineControllerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, rErr)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(isController, tc.IsFalse)
}

// TestIsMachineControllerNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestIsMachineControllerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, coreerrors.NotFound)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineController(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(isController, tc.IsFalse)
}

// TestIsMachineManuallyProvisionedSuccess asserts the happy path of the
// IsMachineManuallyProvisioned service.
func (s *serviceSuite) TestIsMachineManuallyProvisionedSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("666")).Return(true, nil)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineManuallyProvisioned(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isController, tc.IsTrue)
}

// TestIsMachineManuallyProvisionedError asserts that an error coming from the
// state layer is preserved, passed over to the service layer to be maintained
// there.
func (s *serviceSuite) TestIsMachineManuallyProvisionedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("666")).Return(false, rErr)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineManuallyProvisioned(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(isController, tc.IsFalse)
}

// TestIsMachineManuallyProvisionedNotFound asserts that the state layer returns
// a NotFound Error if a machine is not found with the given machineName, and
// that error is preserved and passed on to the service layer to be handled
// there.
func (s *serviceSuite) TestIsMachineManuallyProvisionedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machine.Name("666")).Return(false, coreerrors.NotFound)

	isController, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineManuallyProvisioned(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(isController, tc.IsFalse)
}

func (s *serviceSuite) TestRequireMachineRebootSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RequireMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
}

// TestRequireMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestRequireMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().RequireMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `requiring a machine reboot for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestClearMachineRebootSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ClearMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
}

// TestClearMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestClearMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ClearMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `clear machine reboot flag for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestIsMachineRebootSuccessMachineNeedReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(true, nil)

	needReboot, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needReboot, tc.Equals, true)
}

func (s *serviceSuite) TestIsMachineRebootSuccessMachineDoNotNeedReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(false, nil)

	needReboot, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needReboot, tc.Equals, false)
}

// TestIsMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(false, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `checking if machine with uuid "u-u-i-d" is requiring a reboot: boom`)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of the
// GetMachineParentUUID.
func (s *serviceSuite) TestGetMachineParentUUIDSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("123", nil)

	parentUUID, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(parentUUID, tc.Equals, machine.UUID("123"))
}

// TestGetMachineParentUUIDError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineParentUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", rErr)

	parentUUID, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestGetMachineParentUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", coreerrors.NotFound)

	parentUUID, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestGetMachineParentUUIDMachineHasNoParent asserts that the state layer
// returns a MachineHasNoParent Error if a machine is found with the given
// machineName but has no parent, and that error is preserved and passed on to
// the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDMachineHasNoParent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", machineerrors.MachineHasNoParent)

	parentUUID, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineParentUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineHasNoParent)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestMachineShouldRebootOrShutdownDoNothing asserts that the reboot action is preserved from the state
// layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownDoNothing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldDoNothing, nil)

	needReboot, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needReboot, tc.Equals, machine.ShouldDoNothing)
}

// TestMachineShouldRebootOrShutdownReboot asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldReboot, nil)

	needReboot, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needReboot, tc.Equals, machine.ShouldReboot)
}

// TestMachineShouldRebootOrShutdownShutdown asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownShutdown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldShutdown, nil)

	needReboot, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(needReboot, tc.Equals, machine.ShouldShutdown)
}

// TestMachineShouldRebootOrShutdownError asserts that if the state layer
// returns an Error, this error will be preserved and passed to the service
// layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldDoNothing, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(err, tc.ErrorMatches, `getting if the machine with uuid "u-u-i-d" need to reboot or shutdown: boom`)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of the
// MarkMachineForRemoval service.
func (s *serviceSuite) TestMarkMachineForRemovalSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestMarkMachineForRemovalMachineNotFoundError asserts that the state layer
// returns a MachineNotFound Error if a machine is not found, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestMarkMachineForRemovalMachineNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(machineerrors.MachineNotFound)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMarkMachineForRemovalError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestMarkMachineForRemovalError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Assert(err, tc.ErrorIs, rErr)
}

// TestGetAllMachineRemovalsSuccess asserts the happy path of the
// GetAllMachineRemovals service.
func (s *serviceSuite) TestGetAllMachineRemovalsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return([]machine.UUID{"666"}, nil)

	machineRemovals, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineRemovals, tc.DeepEquals, []machine.UUID{"666"})
}

// TestGetAllMachineRemovalsError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetAllMachineRemovalsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return(nil, rErr)

	machineRemovals, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetAllMachineRemovals(c.Context())
	c.Assert(err, tc.ErrorIs, rErr)
	c.Check(machineRemovals, tc.IsNil)
}

// TestGetMachineUUIDSuccess asserts the happy path of the
// GetMachineUUID.
func (s *serviceSuite) TestGetMachineUUIDSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return("123", nil)

	uuid, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, machine.UUID("123"))
}

// TestGetMachineUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return("", coreerrors.NotFound)

	uuid, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachineUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(uuid, tc.Equals, machine.UUID(""))
}

func (s *serviceSuite) TestLXDProfilesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), machine.UUID("666")).Return([]string{"profile1", "profile2"}, nil)

	profiles, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		AppliedLXDProfileNames(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *serviceSuite) TestLXDProfilesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), machine.UUID("666")).Return(nil, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		AppliedLXDProfileNames(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestSetLXDProfilesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), machine.UUID("666"), []string{"profile1", "profile2"}).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetAppliedLXDProfileNames(c.Context(), machine.UUID("666"), []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLXDProfilesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), machine.UUID("666"), []string{"profile1", "profile2"}).Return(rErr)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetAppliedLXDProfileNames(c.Context(), "666", []string{"profile1", "profile2"})
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestGetAllProvisionedMachineInstanceID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(map[string]string{
		"foo": "123",
	}, nil)

	result, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[machine.Name]instance.Id{
		machine.Name("foo"): instance.Id("123"),
	})
}

func (s *serviceSuite) TestGetAllProvisionedMachineInstanceIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(nil, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetAllProvisionedMachineInstanceID(c.Context())
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestSetMachineHostname(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().SetMachineHostname(gomock.Any(), machineUUID, "new-hostname").Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineHostname(c.Context(), machineUUID, "new-hostname")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetMachineHostnameInvalidMachineUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineHostname(c.Context(), "foo", "new-hostname")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *serviceSuite) TestGetSupportedContainersTypes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetSupportedContainersTypes(gomock.Any(), machineUUID).Return([]string{"lxd"}, nil)

	containerTypes, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetSupportedContainersTypes(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerTypes, tc.DeepEquals, []instance.ContainerType{"lxd"})
}

func (s *serviceSuite) TestGetSupportedContainersTypesInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetSupportedContainersTypes(gomock.Any(), machineUUID).Return([]string{"boo"}, nil)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetSupportedContainersTypes(c.Context(), machineUUID)
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *serviceSuite) TestGetSupportedContainersTypesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID := machinetesting.GenUUID(c)

	s.state.EXPECT().GetSupportedContainersTypes(gomock.Any(), machineUUID).Return([]string{"boo"}, errors.Errorf("boom"))

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetSupportedContainersTypes(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)
}

func (s *serviceSuite) TestGetMachinePrincipalApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	s.state.EXPECT().GetMachinePrincipalApplications(gomock.Any(), machineName).Return([]string{"foo", "bar"}, nil)

	units, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachinePrincipalApplications(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(units, tc.DeepEquals, []string{"foo", "bar"})
}

func (s *serviceSuite) TestGetMachinePrincipalUnitsInvalidMachineUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachinePrincipalApplications(c.Context(), "!!!")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *serviceSuite) TestGetMachinePrincipalUnitsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := machine.Name("0")

	s.state.EXPECT().GetMachinePrincipalApplications(gomock.Any(), machineName).Return([]string{"foo", "bar"}, errors.Errorf("boom"))

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetMachinePrincipalApplications(c.Context(), machineName)
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)
}
