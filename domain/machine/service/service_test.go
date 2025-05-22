// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite
	state *MockState
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestCreateMachineSuccess asserts the happy path of the CreateMachine service.
func (s *serviceSuite) TestCreateMachineSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCreateMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachine(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `creating machine "666": boom`)
}

// TestCreateMachineAlreadyExists asserts that the state layer returns a
// MachineAlreadyExists Error if a machine is already found with the given
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *serviceSuite) TestCreateMachineAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), gomock.Any(), gomock.Any()).Return(machineerrors.MachineAlreadyExists)

	_, err := NewService(s.state).CreateMachine(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestCreateMachineWithParentSuccess asserts the happy path of the
// CreateMachineWithParent service.
func (s *serviceSuite) TestCreateMachineWithParentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("666"), machine.Name("parent"), gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachineWithParent(c.Context(), machine.Name("666"), machine.Name("parent"))
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateMachineWithParentError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCreateMachineWithParentError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("666"), machine.Name("parent"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachineWithParent(c.Context(), machine.Name("666"), machine.Name("parent"))
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `creating machine "666" with parent "parent": boom`)
}

// TestCreateMachineWithParentParentNotFound asserts that the state layer
// returns a NotFound Error if a machine is not found with the given parent
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *serviceSuite) TestCreateMachineWithParentParentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("666"), machine.Name("parent"), gomock.Any(), gomock.Any()).Return(coreerrors.NotFound)

	_, err := NewService(s.state).CreateMachineWithParent(c.Context(), machine.Name("666"), machine.Name("parent"))
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestCreateMachineWithParentMachineAlreadyExists asserts that the state layer
// returns a MachineAlreadyExists Error if a machine is already found with the
// given machineName, and that error is preserved and passed on to the service
// layer to be handled there.
func (s *serviceSuite) TestCreateMachineWithParentMachineAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), machine.Name("666"), machine.Name("parent"), gomock.Any(), gomock.Any()).Return(machineerrors.MachineAlreadyExists)

	_, err := NewService(s.state).CreateMachineWithParent(c.Context(), machine.Name("666"), machine.Name("parent"))
	c.Check(err, tc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestDeleteMachineSuccess asserts the happy path of the DeleteMachine service.
func (s *serviceSuite) TestDeleteMachineSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), machine.Name("666")).Return(nil)

	err := NewService(s.state).DeleteMachine(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
}

// TestDeleteMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestDeleteMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), machine.Name("666")).Return(rErr)

	err := NewService(s.state).DeleteMachine(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `deleting machine "666": boom`)
}

// TestGetLifeSuccess asserts the happy path of the GetMachineLife service.
func (s *serviceSuite) TestGetLifeSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	alive := life.Alive
	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(&alive, nil)

	l, err := NewService(s.state).GetMachineLife(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(l, tc.Equals, &alive)
}

// TestGetLifeError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetLifeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(nil, rErr)

	l, err := NewService(s.state).GetMachineLife(c.Context(), "666")
	c.Check(l, tc.IsNil)
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `getting life status for machine "666": boom`)
}

// TestGetLifeNotFoundError asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetLifeNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineLife(gomock.Any(), machine.Name("666")).Return(nil, coreerrors.NotFound)

	l, err := NewService(s.state).GetMachineLife(c.Context(), "666")
	c.Check(l, tc.IsNil)
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestSetMachineLifeSuccess asserts the happy path of the SetMachineLife
// service.
func (s *serviceSuite) TestSetMachineLifeSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Alive).Return(nil)

	err := NewService(s.state).SetMachineLife(c.Context(), "666", life.Alive)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetMachineLifeError asserts that an error coming from the state layer is
// preserved, and passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineLifeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Alive).Return(rErr)

	err := NewService(s.state).SetMachineLife(c.Context(), "666", life.Alive)
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `setting life status for machine "666": boom`)
}

// TestSetMachineLifeMachineDontExist asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestSetMachineLifeMachineDontExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("nonexistent"), life.Alive).Return(coreerrors.NotFound)

	err := NewService(s.state).SetMachineLife(c.Context(), "nonexistent", life.Alive)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestEnsureDeadMachineSuccess asserts the happy path of the EnsureDeadMachine
// service function.
func (s *serviceSuite) TestEnsureDeadMachineSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Dead).Return(nil)

	err := NewService(s.state).EnsureDeadMachine(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
}

// TestEnsureDeadMachineError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestEnsureDeadMachineError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), machine.Name("666"), life.Dead).Return(rErr)

	err := NewService(s.state).EnsureDeadMachine(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestListAllMachinesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AllMachineNames(gomock.Any()).Return([]machine.Name{"666"}, nil)

	machines, err := NewService(s.state).AllMachineNames(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(machines, tc.DeepEquals, []machine.Name{"666"})
}

// TestListAllMachinesError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestListAllMachinesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AllMachineNames(gomock.Any()).Return(nil, rErr)

	machines, err := NewService(s.state).AllMachineNames(c.Context())
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(machines, tc.IsNil)
}

func (s *serviceSuite) TestInstanceIdSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("123", nil)

	instanceId, err := NewService(s.state).InstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, instance.Id("123"))
}

// TestInstanceIdError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestInstanceIdError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("", rErr)

	instanceId, err := NewService(s.state).InstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(instanceId, tc.Equals, instance.UnknownId)
}

// TestInstanceIdNotProvisionedError asserts that the state layer returns a
// NotProvisioned Error if an instanceId is not found for the given machineName,
// and that error is preserved and passed on to the service layer to be handled
// there.
func (s *serviceSuite) TestInstanceIdNotProvisionedError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceID(gomock.Any(), machine.UUID("deadbeef-0bad-400d-8000-4b1d0d06f00d")).Return("", machineerrors.NotProvisioned)

	instanceId, err := NewService(s.state).InstanceID(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, tc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(instanceId, tc.Equals, instance.UnknownId)
}

// TestGetMachineStatusSuccess asserts the happy path of the GetMachineStatus.
func (s *serviceSuite) TestGetMachineStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := status.StatusInfo{Status: status.Started}
	s.state.EXPECT().GetMachineStatus(gomock.Any(), machine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}, nil)

	machineStatus, err := NewService(s.state).GetMachineStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(machineStatus, tc.DeepEquals, expectedStatus)
}

// TestGetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineStatus(gomock.Any(), machine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, rErr)

	machineStatus, err := NewService(s.state).GetMachineStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(machineStatus, tc.DeepEquals, status.StatusInfo{})
}

// TestSetMachineStatusSuccess asserts the happy path of the SetMachineStatus.
func (s *serviceSuite) TestSetMachineStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Started}
	s.state.EXPECT().SetMachineStatus(gomock.Any(), machine.Name("666"), domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}).Return(nil)

	err := NewService(s.state).SetMachineStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Started}
	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineStatus(gomock.Any(), machine.Name("666"), domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}).Return(rErr)

	err := NewService(s.state).SetMachineStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIs, rErr)
}

// TestSetMachineStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetMachineStatusInvalid(c *tc.C) {
	err := NewService(nil).SetMachineStatus(c.Context(), "666", status.StatusInfo{Status: "invalid"})
	c.Check(err, tc.ErrorIs, machineerrors.InvalidStatus)
}

// TestGetInstanceStatusSuccess asserts the happy path of the GetInstanceStatus.
func (s *serviceSuite) TestGetInstanceStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().GetInstanceStatus(gomock.Any(), machine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}, nil)

	instanceStatus, err := NewService(s.state).GetInstanceStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(instanceStatus, tc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetInstanceStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetInstanceStatus(gomock.Any(), machine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.InstanceStatusType]{}, rErr)

	instanceStatus, err := NewService(s.state).GetInstanceStatus(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(instanceStatus, tc.DeepEquals, status.StatusInfo{})
}

// TestSetInstanceStatusSuccess asserts the happy path of the SetInstanceStatus
// service.
func (s *serviceSuite) TestSetInstanceStatusSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().SetInstanceStatus(gomock.Any(), machine.Name("666"), domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}).Return(nil)

	err := NewService(s.state).SetInstanceStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetInstanceStatusError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	newStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().SetInstanceStatus(gomock.Any(), machine.Name("666"), domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}).Return(rErr)

	err := NewService(s.state).SetInstanceStatus(c.Context(), "666", newStatus)
	c.Check(err, tc.ErrorIs, rErr)
}

// TestSetInstanceStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetInstanceStatusInvalid(c *tc.C) {
	err := NewService(nil).SetInstanceStatus(c.Context(), "666", status.StatusInfo{Status: "invalid"})
	c.Check(err, tc.ErrorIs, machineerrors.InvalidStatus)
}

// TestIsControllerSuccess asserts the happy path of the IsController service.
func (s *serviceSuite) TestIsControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(true, nil)

	isController, err := NewService(s.state).IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIsNil)
	c.Assert(isController, tc.IsTrue)
}

// TestIsControllerError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsControllerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, rErr)

	isController, err := NewService(s.state).IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(isController, tc.IsFalse)
}

// TestIsControllerNotFound asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error
// is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestIsControllerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), machine.Name("666")).Return(false, coreerrors.NotFound)

	isController, err := NewService(s.state).IsMachineController(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(isController, tc.IsFalse)
}

func (s *serviceSuite) TestRequireMachineRebootSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RequireMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(nil)

	err := NewService(s.state).RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
}

// TestRequireMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestRequireMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().RequireMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(rErr)

	err := NewService(s.state).RequireMachineReboot(c.Context(), "u-u-i-d")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `requiring a machine reboot for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestClearMachineRebootSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ClearMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(nil)

	err := NewService(s.state).ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
}

// TestClearMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestClearMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ClearMachineReboot(gomock.Any(), machine.UUID("u-u-i-d")).Return(rErr)

	err := NewService(s.state).ClearMachineReboot(c.Context(), "u-u-i-d")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `clear machine reboot flag for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestIsMachineRebootSuccessMachineNeedReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(true, nil)

	needReboot, err := NewService(s.state).IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needReboot, tc.Equals, true)
}

func (s *serviceSuite) TestIsMachineRebootSuccessMachineDontNeedReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(false, nil)

	needReboot, err := NewService(s.state).IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needReboot, tc.Equals, false)
}

// TestIsMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsMachineRebootError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), machine.UUID("u-u-i-d")).Return(false, rErr)

	_, err := NewService(s.state).IsMachineRebootRequired(c.Context(), "u-u-i-d")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `checking if machine with uuid "u-u-i-d" is requiring a reboot: boom`)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of the
// GetMachineParentUUID.
func (s *serviceSuite) TestGetMachineParentUUIDSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("123", nil)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(parentUUID, tc.Equals, machine.UUID("123"))
}

// TestGetMachineParentUUIDError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineParentUUIDError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", rErr)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestGetMachineParentUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", coreerrors.NotFound)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(c.Context(), machine.UUID("666"))
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestGetMachineParentUUIDMachineHasNoParent asserts that the state layer
// returns a MachineHasNoParent Error if a machine is found with the given
// machineName but has no parent, and that error is preserved and passed on to
// the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDMachineHasNoParent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), machine.UUID("666")).Return("", machineerrors.MachineHasNoParent)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(c.Context(), "666")
	c.Check(err, tc.ErrorIs, machineerrors.MachineHasNoParent)
	c.Check(parentUUID, tc.Equals, machine.UUID(""))
}

// TestMachineShouldRebootOrShutdownDoNothing asserts that the reboot action is preserved from the state
// layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownDoNothing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldDoNothing, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needReboot, tc.Equals, machine.ShouldDoNothing)
}

// TestMachineShouldRebootOrShutdownReboot asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldReboot, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needReboot, tc.Equals, machine.ShouldReboot)
}

// TestMachineShouldRebootOrShutdownShutdown asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownShutdown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldShutdown, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(needReboot, tc.Equals, machine.ShouldShutdown)
}

// TestMachineShouldRebootOrShutdownError asserts that if the state layer
// returns an Error, this error will be preserved and passed to the service
// layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), machine.UUID("u-u-i-d")).Return(machine.ShouldDoNothing, rErr)

	_, err := NewService(s.state).ShouldRebootOrShutdown(c.Context(), "u-u-i-d")
	c.Check(err, tc.ErrorIs, rErr)
	c.Assert(err, tc.ErrorMatches, `getting if the machine with uuid "u-u-i-d" need to reboot or shutdown: boom`)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of the
// MarkMachineForRemoval service.
func (s *serviceSuite) TestMarkMachineForRemovalSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(nil)

	err := NewService(s.state).MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIsNil)
}

// TestMarkMachineForRemovalMachineNotFoundError asserts that the state layer
// returns a MachineNotFound Error if a machine is not found, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestMarkMachineForRemovalMachineNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(machineerrors.MachineNotFound)

	err := NewService(s.state).MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMarkMachineForRemovalError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestMarkMachineForRemovalError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), machine.Name("666")).Return(rErr)

	err := NewService(s.state).MarkMachineForRemoval(c.Context(), machine.Name("666"))
	c.Check(err, tc.ErrorIs, rErr)
}

// TestGetAllMachineRemovalsSuccess asserts the happy path of the
// GetAllMachineRemovals service.
func (s *serviceSuite) TestGetAllMachineRemovalsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return([]machine.UUID{"666"}, nil)

	machineRemovals, err := NewService(s.state).GetAllMachineRemovals(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(machineRemovals, tc.DeepEquals, []machine.UUID{"666"})
}

// TestGetAllMachineRemovalsError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetAllMachineRemovalsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return(nil, rErr)

	machineRemovals, err := NewService(s.state).GetAllMachineRemovals(c.Context())
	c.Check(err, tc.ErrorIs, rErr)
	c.Check(machineRemovals, tc.IsNil)
}

// TestGetMachineUUIDSuccess asserts the happy path of the
// GetMachineUUID.
func (s *serviceSuite) TestGetMachineUUIDSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return("123", nil)

	uuid, err := NewService(s.state).GetMachineUUID(c.Context(), "666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, machine.UUID("123"))
}

// TestGetMachineUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("666")).Return("", coreerrors.NotFound)

	uuid, err := NewService(s.state).GetMachineUUID(c.Context(), "666")
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
	c.Check(uuid, tc.Equals, machine.UUID(""))
}

func (s *serviceSuite) TestLXDProfilesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), machine.UUID("666")).Return([]string{"profile1", "profile2"}, nil)

	profiles, err := NewService(s.state).AppliedLXDProfileNames(c.Context(), "666")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(profiles, tc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *serviceSuite) TestLXDProfilesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), machine.UUID("666")).Return(nil, rErr)

	_, err := NewService(s.state).AppliedLXDProfileNames(c.Context(), "666")
	c.Check(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestSetLXDProfilesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), machine.UUID("666"), []string{"profile1", "profile2"}).Return(nil)

	err := NewService(s.state).SetAppliedLXDProfileNames(c.Context(), machine.UUID("666"), []string{"profile1", "profile2"})
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLXDProfilesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), machine.UUID("666"), []string{"profile1", "profile2"}).Return(rErr)

	err := NewService(s.state).SetAppliedLXDProfileNames(c.Context(), "666", []string{"profile1", "profile2"})
	c.Check(err, tc.ErrorIs, rErr)
}
