// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
)

func (s *serviceSuite) TestRetrieveHardwareCharacteristics(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	s.state.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("42")).
		Return(expected, nil)

	hc, err := NewService(s.state).HardwareCharacteristics(context.Background(), "42")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hc, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestRetrieveHardwareCharacteristicsFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().HardwareCharacteristics(gomock.Any(), machine.UUID("42")).
		Return(nil, errors.New("boom"))

	hc, err := NewService(s.state).HardwareCharacteristics(context.Background(), "42")
	c.Check(hc, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "retrieving hardware characteristics for machine \"42\": boom")
}

func (s *serviceSuite) TestRetrieveAvailabilityZone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AvailabilityZone(gomock.Any(), machine.UUID("42")).
		Return("foo", nil)

	hc, err := NewService(s.state).AvailabilityZone(context.Background(), "42")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hc, tc.DeepEquals, "foo")
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
		machine.UUID("42"),
		instance.Id("instance-42"),
		"42",
		hc,
	).Return(nil)

	err := NewService(s.state).SetMachineCloudInstance(context.Background(), "42", "instance-42", "42", hc)
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
		machine.UUID("42"),
		instance.Id("instance-42"),
		"42",
		hc,
	).Return(errors.New("boom"))

	err := NewService(s.state).SetMachineCloudInstance(context.Background(), "42", "instance-42", "42", hc)
	c.Assert(err, tc.ErrorMatches, "setting machine cloud instance for machine \"42\": boom")
}

func (s *serviceSuite) TestDeleteMachineCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachineCloudInstance(gomock.Any(), machine.UUID("42")).Return(nil)

	err := NewService(s.state).DeleteMachineCloudInstance(context.Background(), "42")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteMachineCloudInstanceFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachineCloudInstance(gomock.Any(), machine.UUID("42")).Return(errors.New("boom"))

	err := NewService(s.state).DeleteMachineCloudInstance(context.Background(), "42")
	c.Assert(err, tc.ErrorMatches, "deleting machine cloud instance for machine \"42\": boom")
}

func uintptr(u uint64) *uint64 {
	return &u
}
