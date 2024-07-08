// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	cmachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/life"
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

func (s *serviceSuite) TestCreateMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CreateMachine(gomock.Any(), cmachine.Name("666"), gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachine(context.Background(), cmachine.Name("666"))
	c.Assert(err, jc.ErrorIsNil)
}

// TestCreateError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCreateMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), cmachine.Name("666"), gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachine(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating machine "666": boom`)
}

func (s *serviceSuite) TestDeleteMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), cmachine.Name("666")).Return(nil)

	err := NewService(s.state).DeleteMachine(context.Background(), cmachine.Name("666"))
	c.Assert(err, jc.ErrorIsNil)
}

// TestDeleteMachineError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestDeleteMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), cmachine.Name("666")).Return(rErr)

	err := NewService(s.state).DeleteMachine(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting machine "666": boom`)
}

func (s *serviceSuite) TestGetLifeSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	life := life.Alive
	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(&life, nil)

	l, err := NewService(s.state).GetMachineLife(context.Background(), cmachine.Name("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(l, gc.Equals, &life)
}

// TestGetLifeError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestGetLifeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(nil, rErr)

	l, err := NewService(s.state).GetMachineLife(context.Background(), cmachine.Name("666"))
	c.Check(l, gc.IsNil)
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `getting life status for machine "666": boom`)
}

// TestGetLifeNotFoundError asserts that the state layer returns a NotFound
// Error if a machine is not found with the given machineName, and that error is
// preserved and passed on to the service layer to be handled there.
func (s *serviceSuite) TestGetLifeNotFoundError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineLife(gomock.Any(), cmachine.Name("666")).Return(nil, errors.NotFound)

	l, err := NewService(s.state).GetMachineLife(context.Background(), cmachine.Name("666"))
	c.Check(l, gc.IsNil)
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestListAllMachinesSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AllMachineNames(gomock.Any()).Return([]cmachine.Name{cmachine.Name("666")}, nil)

	machines, err := NewService(s.state).AllMachineNames(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []cmachine.Name{cmachine.Name("666")})
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

	s.state.EXPECT().InstanceId(gomock.Any(), cmachine.Name("666")).Return("123", nil)

	instanceId, err := NewService(s.state).InstanceId(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(instanceId, gc.Equals, "123")
}

// TestInstanceIdError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestInstanceIdError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().InstanceId(gomock.Any(), cmachine.Name("666")).Return("", rErr)

	instanceId, err := NewService(s.state).InstanceId(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(instanceId, gc.Equals, "")
}

// TestInstanceIdNotProvisionedError asserts that the state layer returns a
// NotProvisioned Error if an instanceId is not found for the given machineName,
// and that error is preserved and passed on to the service layer to be handled
// there.
func (s *serviceSuite) TestInstanceIdNotProvisionedError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceId(gomock.Any(), cmachine.Name("666")).Return("", errors.NotProvisioned)

	instanceId, err := NewService(s.state).InstanceId(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, errors.NotProvisioned)
	c.Check(instanceId, gc.Equals, "")
}

func (s *serviceSuite) TestInstanceStatusSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().InstanceStatus(gomock.Any(), cmachine.Name("666")).Return("running", nil)

	instanceStatus, err := NewService(s.state).InstanceStatus(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(instanceStatus, gc.Equals, "running")
}

// TestInstanceStatusError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestInstanceStatusError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().InstanceStatus(gomock.Any(), cmachine.Name("666")).Return("", rErr)

	instanceStatus, err := NewService(s.state).InstanceStatus(context.Background(), cmachine.Name("666"))
	c.Check(err, jc.ErrorIs, rErr)
	c.Check(instanceStatus, gc.Equals, "")
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

func (s *serviceSuite) TestCancelMachineRebootSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CancelMachineReboot(gomock.Any(), "u-u-i-d").Return(nil)

	err := NewService(s.state).CancelMachineReboot(context.Background(), "u-u-i-d")
	c.Assert(err, jc.ErrorIsNil)
}

// TestCancelMachineRebootError asserts that an error coming from the state layer is
// preserved, passed over to the service layer to be maintained there.
func (s *serviceSuite) TestCancelMachineRebootError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CancelMachineReboot(gomock.Any(), "u-u-i-d").Return(rErr)

	err := NewService(s.state).CancelMachineReboot(context.Background(), "u-u-i-d")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `cancelling a machine reboot for machine with uuid "u-u-i-d": boom`)
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
