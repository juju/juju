// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/clock"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	domainmachine "github.com/juju/juju/domain/machine"
	domainmachineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *serviceSuite) TestRetrieveHardwareCharacteristicsMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	s.state.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUID.String()).
		Return(instance.HardwareCharacteristics{}, domainmachineerrors.MachineNotFound)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	_, err := svc.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, domainmachineerrors.MachineNotFound)
}

func (s *serviceSuite) TestRetrieveHardwareCharacteristics(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	expected := instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	s.state.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUID.String()).
		Return(expected, nil)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	hc, err := svc.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hc, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestRetrieveHardwareCharacteristicsFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	opErr := errors.New("boom")
	s.state.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUID.String()).
		Return(instance.HardwareCharacteristics{}, opErr)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	_, err := svc.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, opErr)
}

func (s *serviceSuite) TestRetrieveAvailabilityZoneMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	s.state.EXPECT().AvailabilityZone(gomock.Any(), machineUUID.String()).
		Return("", domainmachineerrors.MachineNotFound)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	_, err := svc.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, domainmachineerrors.MachineNotFound)
}

func (s *serviceSuite) TestRetrieveAvailabilityZoneNotSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	s.state.EXPECT().AvailabilityZone(gomock.Any(), machineUUID.String()).
		Return("", domainmachineerrors.AvailabilityZoneNotFound)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	_, err := svc.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, domainmachineerrors.AvailabilityZoneNotFound)
}

func (s *serviceSuite) TestRetrieveAvailabilityZone(c *tc.C) {
	defer s.setupMocks(c).Finish()
	machineUUID := tc.Must(c, coremachine.NewUUID)

	s.state.EXPECT().AvailabilityZone(gomock.Any(), machineUUID.String()).
		Return("foo", nil)

	svc := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c))
	hc, err := svc.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hc, tc.Equals, "foo")
}

func (s *serviceSuite) TestSetMachineCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hc := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	s.state.EXPECT().SetMachineCloudInstance(
		gomock.Any(),
		"42",
		instance.Id("instance-42"),
		"42",
		"nonce",
		hc,
	).Return(nil)

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineCloudInstance(c.Context(), "42", "instance-42", "42", "nonce", hc)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetMachineCloudInstanceFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hc := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	s.state.EXPECT().SetMachineCloudInstance(
		gomock.Any(),
		"42",
		instance.Id("instance-42"),
		"42",
		"nonce",
		hc,
	).Return(errors.New("boom"))

	err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		SetMachineCloudInstance(c.Context(), "42", "instance-42", "42", "nonce", hc)
	c.Assert(err, tc.ErrorMatches, "setting machine cloud instance for machine \"42\": boom")
}

func (s *serviceSuite) TestGetPollingInfosSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	n1 := coremachine.Name("0")
	n2 := coremachine.Name("1")
	expected := domainmachine.PollingInfos{{
		MachineUUID:         "uuid-0",
		MachineName:         n1,
		InstanceID:          "i-0",
		ExistingDeviceCount: 1,
	}, {
		MachineUUID:         "uuid-1",
		MachineName:         n2,
		InstanceID:          "",
		ExistingDeviceCount: 0,
	}}

	s.state.EXPECT().GetPollingInfos(gomock.Any(), []string{"0", "1"}).Return(expected, nil)

	infos, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetPollingInfos(c.Context(), []coremachine.Name{n1, n2})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(infos, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetPollingInfosValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// invalid machine name should cause validation error and short-circuit before state call
	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetPollingInfos(c.Context(), []coremachine.Name{"invalid"})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetPollingInfosStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	n := coremachine.Name("0")
	rErr := errors.New("boom")
	s.state.EXPECT().GetPollingInfos(gomock.Any(), []string{"0"}).Return(nil, rErr)

	_, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetPollingInfos(c.Context(), []coremachine.Name{n})
	c.Assert(err, tc.ErrorIs, rErr)
}

func (s *serviceSuite) TestGetPollingInfosEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	result, err := NewService(s.state, s.statusHistory, clock.WallClock, loggertesting.WrapCheckLog(c)).
		GetPollingInfos(c.Context(), []coremachine.Name{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

func uintptr(u uint64) *uint64 {
	return &u
}
