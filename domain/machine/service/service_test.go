// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	cmachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.IsolationSuite
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestCreateMachineSuccess asserts the happy path of the CreateMachine service.
func (s *serviceSuite) TestCreateMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), cmachine.Name("666"), gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCreateMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), cmachine.Name("666"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating machine "666": boom`)
}

// TestCreateMachineAlreadyExists asserts that the state layer returns a
// MachineAlreadyExists Error if a machine is already found with the given
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *serviceSuite) TestCreateMachineAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), cmachine.Name("666"), gomock.Any(), gomock.Any()).Return(machineerrors.MachineAlreadyExists)

	_, err := NewService(s.state).CreateMachine(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestCreateMachineWithParentSuccess asserts the happy path of the
// CreateMachineWithParent service.
func (s *serviceSuite) TestCreateMachineWithParentSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), cmachine.Name("666"), cmachine.Name("parent"), gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachineWithParent(context.Background(), cmachine.Name("666"), cmachine.Name("parent"))
	c.Assert(err, jc.ErrorIsNil)
}

// TestCreateMachineWithParentError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCreateMachineWithParentError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), cmachine.Name("666"), cmachine.Name("parent"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachineWithParent(context.Background(), cmachine.Name("666"), cmachine.Name("parent"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating machine "666" with parent "parent": boom`)
}

// TestCreateMachineWithParentParentNotFound asserts that the state layer
// returns a NotFound Error if a machine is not found with the given parent
// machineName, and that error is preserved and passed on to the service layer
// to be handled there.
func (s *serviceSuite) TestCreateMachineWithParentParentNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), cmachine.Name("666"), cmachine.Name("parent"), gomock.Any(), gomock.Any()).Return(coreerrors.NotFound)

	_, err := NewService(s.state).CreateMachineWithParent(context.Background(), cmachine.Name("666"), cmachine.Name("parent"))
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
}

// TestCreateMachineWithParentMachineAlreadyExists asserts that the state layer
// returns a MachineAlreadyExists Error if a machine is already found with the
// given machineName, and that error is preserved and passed on to the service
// layer to be handled there.
func (s *serviceSuite) TestCreateMachineWithParentMachineAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachineWithParent(gomock.Any(), cmachine.Name("666"), cmachine.Name("parent"), gomock.Any(), gomock.Any()).Return(machineerrors.MachineAlreadyExists)

	_, err := NewService(s.state).CreateMachineWithParent(context.Background(), cmachine.Name("666"), cmachine.Name("parent"))
	c.Check(err, jc.ErrorIs, machineerrors.MachineAlreadyExists)
}

// TestDeleteMachineSuccess asserts the happy path of the DeleteMachine service.
func (s *serviceSuite) TestDeleteMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), cmachine.Name("666")).Return(nil)

	err := NewService(s.state).DeleteMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

// TestDeleteMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestDeleteMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), cmachine.Name("666")).Return(rErr)

	err := NewService(s.state).DeleteMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting machine "666": boom`)
}

// TestGetLifeSuccess asserts the happy path of the GetMachineLife service.
func (s *serviceSuite) TestGetLifeSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	alive := life.Alive
	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(&alive, nil)

	l, err := NewService(s.state).GetMachineLife(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(l, gc.Equals, &alive)
}

// TestGetLifeError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetLifeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(nil, rErr)

	l, err := NewService(s.state).GetMachineLife(context.Background(), "666")
	c.Check(l, gc.IsNil)
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `getting life status for machine "666": boom`)
}

// TestGetLifeNotFoundError asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetLifeNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(nil, coreerrors.NotFound)

	l, err := NewService(s.state).GetMachineLife(context.Background(), "666")
	c.Check(l, gc.IsNil)
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
}

// TestSetMachineLifeSuccess asserts the happy path of the SetMachineLife
// service.
func (s *serviceSuite) TestSetMachineLifeSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), cmachine.Name("666"), life.Alive).Return(nil)

	err := NewService(s.state).SetMachineLife(context.Background(), "666", life.Alive)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetMachineLifeError asserts that an error coming from the state layer is
// preserved, and passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineLifeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), cmachine.Name("666"), life.Alive).Return(rErr)

	err := NewService(s.state).SetMachineLife(context.Background(), "666", life.Alive)
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `setting life status for machine "666": boom`)
}

// TestSetMachineLifeMachineDontExist asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestSetMachineLifeMachineDontExist(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), cmachine.Name("nonexistent"), life.Alive).Return(coreerrors.NotFound)

	err := NewService(s.state).SetMachineLife(context.Background(), "nonexistent", life.Alive)
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

// TestEnsureDeadMachineSuccess asserts the happy path of the EnsureDeadMachine
// service function.
func (s *serviceSuite) TestEnsureDeadMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetMachineLife(gomock.Any(), cmachine.Name("666"), life.Dead).Return(nil)

	err := NewService(s.state).EnsureDeadMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIsNil)
}

// TestEnsureDeadMachineError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestEnsureDeadMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineLife(gomock.Any(), cmachine.Name("666"), life.Dead).Return(rErr)

	err := NewService(s.state).EnsureDeadMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
}

func (s *serviceSuite) TestListAllMachinesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AllMachineNames(gomock.Any()).Return([]cmachine.Name{"666"}, nil)

	machines, err := NewService(s.state).AllMachineNames(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []cmachine.Name{"666"})
}

// TestListAllMachinesError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestListAllMachinesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AllMachineNames(gomock.Any()).Return(nil, rErr)

	machines, err := NewService(s.state).AllMachineNames(context.Background())
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(machines, gc.IsNil)
}

func (s *serviceSuite) TestInstanceIdSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceID(gomock.Any(), "deadbeef-0bad-400d-8000-4b1d0d06f00d").Return("123", nil)

	instanceId, err := NewService(s.state).InstanceID(context.Background(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, jc.ErrorIsNil)
	c.Check(instanceId, gc.Equals, instance.Id("123"))
}

// TestInstanceIdError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestInstanceIdError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().InstanceID(gomock.Any(), "deadbeef-0bad-400d-8000-4b1d0d06f00d").Return("", rErr)

	instanceId, err := NewService(s.state).InstanceID(context.Background(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(instanceId, gc.Equals, instance.UnknownId)
}

// TestInstanceIdNotProvisionedError asserts that the state layer returns a
// NotProvisioned Error if an instanceId is not found for the given machineName,
// and that error is preserved and passed on to the service layer to be handled
// there.
func (s *serviceSuite) TestInstanceIdNotProvisionedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceID(gomock.Any(), "deadbeef-0bad-400d-8000-4b1d0d06f00d").Return("", machineerrors.NotProvisioned)

	instanceId, err := NewService(s.state).InstanceID(context.Background(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(err, jc.ErrorIs, machineerrors.NotProvisioned)
	c.Check(instanceId, gc.Equals, instance.UnknownId)
}

// TestGetMachineStatusSuccess asserts the happy path of the GetMachineStatus.
func (s *serviceSuite) TestGetMachineStatusSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := status.StatusInfo{Status: status.Started}
	s.state.EXPECT().GetMachineStatus(gomock.Any(), cmachine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}, nil)

	machineStatus, err := NewService(s.state).GetMachineStatus(context.Background(), "666")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(machineStatus, gc.DeepEquals, expectedStatus)
}

// TestGetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineStatusError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineStatus(gomock.Any(), cmachine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.MachineStatusType]{}, rErr)

	machineStatus, err := NewService(s.state).GetMachineStatus(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(machineStatus, gc.DeepEquals, status.StatusInfo{})
}

// TestSetMachineStatusSuccess asserts the happy path of the SetMachineStatus.
func (s *serviceSuite) TestSetMachineStatusSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Started}
	s.state.EXPECT().SetMachineStatus(gomock.Any(), cmachine.Name("666"), domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}).Return(nil)

	err := NewService(s.state).SetMachineStatus(context.Background(), "666", newStatus)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetMachineStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetMachineStatusError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Started}
	rErr := errors.New("boom")
	s.state.EXPECT().SetMachineStatus(gomock.Any(), cmachine.Name("666"), domainmachine.StatusInfo[domainmachine.MachineStatusType]{
		Status: domainmachine.MachineStatusStarted,
	}).Return(rErr)

	err := NewService(s.state).SetMachineStatus(context.Background(), "666", newStatus)
	c.Check(err, jc.ErrorIs, rErr)
}

// TestSetMachineStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetMachineStatusInvalid(c *gc.C) {
	err := NewService(nil).SetMachineStatus(context.Background(), "666", status.StatusInfo{Status: "invalid"})
	c.Check(err, jc.ErrorIs, machineerrors.InvalidStatus)
}

// TestGetInstanceStatusSuccess asserts the happy path of the GetInstanceStatus.
func (s *serviceSuite) TestGetInstanceStatusSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().GetInstanceStatus(gomock.Any(), cmachine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}, nil)

	instanceStatus, err := NewService(s.state).GetInstanceStatus(context.Background(), "666")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(instanceStatus, gc.DeepEquals, expectedStatus)
}

// TestGetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetInstanceStatusError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetInstanceStatus(gomock.Any(), cmachine.Name("666")).Return(domainmachine.StatusInfo[domainmachine.InstanceStatusType]{}, rErr)

	instanceStatus, err := NewService(s.state).GetInstanceStatus(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(instanceStatus, gc.DeepEquals, status.StatusInfo{})
}

// TestSetInstanceStatusSuccess asserts the happy path of the SetInstanceStatus
// service.
func (s *serviceSuite) TestSetInstanceStatusSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	newStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().SetInstanceStatus(gomock.Any(), cmachine.Name("666"), domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}).Return(nil)

	err := NewService(s.state).SetInstanceStatus(context.Background(), "666", newStatus)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetInstanceStatusError asserts that an error coming from the state layer
// is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestSetInstanceStatusError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	newStatus := status.StatusInfo{Status: status.Running}
	s.state.EXPECT().SetInstanceStatus(gomock.Any(), cmachine.Name("666"), domainmachine.StatusInfo[domainmachine.InstanceStatusType]{
		Status: domainmachine.InstanceStatusRunning,
	}).Return(rErr)

	err := NewService(s.state).SetInstanceStatus(context.Background(), "666", newStatus)
	c.Check(err, jc.ErrorIs, rErr)
}

// TestSetInstanceStatusInvalid asserts that an invalid status is passed to the
// service will result in a InvalidStatus error.
func (s *serviceSuite) TestSetInstanceStatusInvalid(c *gc.C) {
	err := NewService(nil).SetInstanceStatus(context.Background(), "666", status.StatusInfo{Status: "invalid"})
	c.Check(err, jc.ErrorIs, machineerrors.InvalidStatus)
}

// TestIsControllerSuccess asserts the happy path of the IsController service.
func (s *serviceSuite) TestIsControllerSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), cmachine.Name("666")).Return(true, nil)

	isController, err := NewService(s.state).IsMachineController(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(isController, jc.IsTrue)
}

// TestIsControllerError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsControllerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineController(gomock.Any(), cmachine.Name("666")).Return(false, rErr)

	isController, err := NewService(s.state).IsMachineController(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(isController, jc.IsFalse)
}

// TestIsControllerNotFound asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error
// is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestIsControllerNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineController(gomock.Any(), cmachine.Name("666")).Return(false, coreerrors.NotFound)

	isController, err := NewService(s.state).IsMachineController(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
	c.Check(isController, jc.IsFalse)
}

func (s *serviceSuite) TestRequireMachineRebootSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().RequireMachineReboot(gomock.Any(), "u-u-i-d").Return(nil)

	err := NewService(s.state).RequireMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
}

// TestRequireMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestRequireMachineRebootError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().RequireMachineReboot(gomock.Any(), "u-u-i-d").Return(rErr)

	err := NewService(s.state).RequireMachineReboot(context.Background(), "u-u-i-d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `requiring a machine reboot for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestClearMachineRebootSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ClearMachineReboot(gomock.Any(), "u-u-i-d").Return(nil)

	err := NewService(s.state).ClearMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
}

// TestClearMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestClearMachineRebootError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ClearMachineReboot(gomock.Any(), "u-u-i-d").Return(rErr)

	err := NewService(s.state).ClearMachineReboot(context.Background(), "u-u-i-d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `clear machine reboot flag for machine with uuid "u-u-i-d": boom`)
}

func (s *serviceSuite) TestIsMachineRebootSuccessMachineNeedReboot(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), "u-u-i-d").Return(true, nil)

	needReboot, err := NewService(s.state).IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needReboot, gc.Equals, true)
}
func (s *serviceSuite) TestIsMachineRebootSuccessMachineDontNeedReboot(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), "u-u-i-d").Return(false, nil)

	needReboot, err := NewService(s.state).IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needReboot, gc.Equals, false)
}

// TestIsMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestIsMachineRebootError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().IsMachineRebootRequired(gomock.Any(), "u-u-i-d").Return(false, rErr)

	_, err := NewService(s.state).IsMachineRebootRequired(context.Background(), "u-u-i-d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `checking if machine with uuid "u-u-i-d" is requiring a reboot: boom`)
}

// TestGetMachineParentUUIDSuccess asserts the happy path of the
// GetMachineParentUUID.
func (s *serviceSuite) TestGetMachineParentUUIDSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), "666").Return("123", nil)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parentUUID, gc.Equals, "123")
}

// TestGetMachineParentUUIDError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetMachineParentUUIDError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), "666").Return("", rErr)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(parentUUID, gc.Equals, "")
}

// TestGetMachineParentUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), "666").Return("", coreerrors.NotFound)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(context.Background(), "666")
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
	c.Check(parentUUID, gc.Equals, "")
}

// TestGetMachineParentUUIDMachineHasNoParent asserts that the state layer
// returns a MachineHasNoParent Error if a machine is found with the given
// machineName but has no parent, and that error is preserved and passed on to
// the service layer to be handled there.
func (s *serviceSuite) TestGetMachineParentUUIDMachineHasNoParent(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineParentUUID(gomock.Any(), "666").Return("", machineerrors.MachineHasNoParent)

	parentUUID, err := NewService(s.state).GetMachineParentUUID(context.Background(), "666")
	c.Check(err, jc.ErrorIs, machineerrors.MachineHasNoParent)
	c.Check(parentUUID, gc.Equals, "")
}

// TestMachineShouldRebootOrShutdownDoNothing asserts that the reboot action is preserved from the state
// layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownDoNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), "u-u-i-d").Return(cmachine.ShouldDoNothing, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needReboot, gc.Equals, cmachine.ShouldDoNothing)
}

// TestMachineShouldRebootOrShutdownReboot asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownReboot(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), "u-u-i-d").Return(cmachine.ShouldReboot, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needReboot, gc.Equals, cmachine.ShouldReboot)
}

// TestMachineShouldRebootOrShutdownShutdown asserts that the reboot action is
// preserved from the state layer through the service layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownShutdown(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), "u-u-i-d").Return(cmachine.ShouldShutdown, nil)

	needReboot, err := NewService(s.state).ShouldRebootOrShutdown(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(needReboot, gc.Equals, cmachine.ShouldShutdown)
}

// TestMachineShouldRebootOrShutdownError asserts that if the state layer
// returns an Error, this error will be preserved and passed to the service
// layer.
func (s *serviceSuite) TestMachineShouldRebootOrShutdownError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().ShouldRebootOrShutdown(gomock.Any(), "u-u-i-d").Return(cmachine.ShouldDoNothing, rErr)

	_, err := NewService(s.state).ShouldRebootOrShutdown(context.Background(), "u-u-i-d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `getting if the machine with uuid "u-u-i-d" need to reboot or shutdown: boom`)
}

// TestMarkMachineForRemovalSuccess asserts the happy path of the
// MarkMachineForRemoval service.
func (s *serviceSuite) TestMarkMachineForRemovalSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), cmachine.Name("666")).Return(nil)

	err := NewService(s.state).MarkMachineForRemoval(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIsNil)
}

// TestMarkMachineForRemovalMachineNotFoundError asserts that the state layer
// returns a MachineNotFound Error if a machine is not found, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestMarkMachineForRemovalMachineNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), cmachine.Name("666")).Return(machineerrors.MachineNotFound)

	err := NewService(s.state).MarkMachineForRemoval(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMarkMachineForRemovalError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestMarkMachineForRemovalError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().MarkMachineForRemoval(gomock.Any(), cmachine.Name("666")).Return(rErr)

	err := NewService(s.state).MarkMachineForRemoval(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
}

// TestGetAllMachineRemovalsSuccess asserts the happy path of the
// GetAllMachineRemovals service.
func (s *serviceSuite) TestGetAllMachineRemovalsSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return([]string{"666"}, nil)

	machineRemovals, err := NewService(s.state).GetAllMachineRemovals(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(machineRemovals, gc.DeepEquals, []string{"666"})
}

// TestGetAllMachineRemovalsError asserts that an error coming from the state
// layer is preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetAllMachineRemovalsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetAllMachineRemovals(gomock.Any()).Return(nil, rErr)

	machineRemovals, err := NewService(s.state).GetAllMachineRemovals(context.Background())
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(machineRemovals, gc.IsNil)
}

// TestGetMachineUUIDSuccess asserts the happy path of the
// GetMachineUUID.
func (s *serviceSuite) TestGetMachineUUIDSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), cmachine.Name("666")).Return("123", nil)

	uuid, err := NewService(s.state).GetMachineUUID(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, "123")
}

// TestGetMachineUUIDNotFound asserts that the state layer returns a
// NotFound Error if a machine is not found with the given machineName, and that
// error is preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetMachineUUIDNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUID(gomock.Any(), cmachine.Name("666")).Return("", coreerrors.NotFound)

	uuid, err := NewService(s.state).GetMachineUUID(context.Background(), "666")
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
	c.Check(uuid, gc.Equals, "")
}

func (s *serviceSuite) TestLXDProfilesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), "666").Return([]string{"profile1", "profile2"}, nil)

	profiles, err := NewService(s.state).AppliedLXDProfileNames(context.Background(), "666")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(profiles, gc.DeepEquals, []string{"profile1", "profile2"})
}

func (s *serviceSuite) TestLXDProfilesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().AppliedLXDProfileNames(gomock.Any(), "666").Return(nil, rErr)

	_, err := NewService(s.state).AppliedLXDProfileNames(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
}

func (s *serviceSuite) TestSetLXDProfilesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), "666", []string{"profile1", "profile2"}).Return(nil)

	err := NewService(s.state).SetAppliedLXDProfileNames(context.Background(), "666", []string{"profile1", "profile2"})
	c.Check(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetLXDProfilesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().SetAppliedLXDProfileNames(gomock.Any(), "666", []string{"profile1", "profile2"}).Return(rErr)

	err := NewService(s.state).SetAppliedLXDProfileNames(context.Background(), "666", []string{"profile1", "profile2"})
	c.Check(err, jc.ErrorIs, rErr)
}

// TestSetReportedMachineAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedMachineAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *serviceSuite) TestSetReportedMachineAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).SetReportedMachineAgentVersion(
		context.Background(),
		cmachine.Name("0"),
		coreagentbinary.Version{
			Number: version.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedMachineAgentVersionSuccess asserts that if we try to set the
// reported agent version for a machine that doesn't exist we get an error
// satisfying [machineerrors.MachineNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *serviceSuite) TestSetReportedMachineAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.state.EXPECT().GetMachineUUID(gomock.Any(), cmachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	err := NewService(s.state).SetReportedMachineAgentVersion(
		context.Background(),
		cmachine.Name("0"),
		coreagentbinary.Version{
			Number: version.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)

	// MachineNotFound error location 2.
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), cmachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: version.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineNotFound)

	err = NewService(s.state).SetReportedMachineAgentVersion(
		context.Background(),
		cmachine.Name("0"),
		coreagentbinary.Version{
			Number: version.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetReportedMachineAgentVersion asserts the happy path of
// [Service.SetReportedMachineAgentVersion].
func (s *serviceSuite) TestSetReportedMachineAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUID(gomock.Any(), cmachine.Name("0")).Return(
		machineUUID.String(), nil,
	)
	s.state.EXPECT().SetRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: version.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err = NewService(s.state).SetReportedMachineAgentVersion(
		context.Background(),
		cmachine.Name("0"),
		coreagentbinary.Version{
			Number: version.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}
