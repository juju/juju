// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/errors"
)

func (s *serviceSuite) TestRetrieveHardwareCharacteristics(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expected := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	s.state.EXPECT().HardwareCharacteristics(gomock.Any(), "42").
		Return(expected, nil)

	hc, err := NewService(s.state).HardwareCharacteristics(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hc, gc.DeepEquals, expected)
}

func (s *serviceSuite) TestRetrieveHardwareCharacteristicsFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().HardwareCharacteristics(gomock.Any(), "42").
		Return(nil, errors.New("boom"))

	hc, err := NewService(s.state).HardwareCharacteristics(context.Background(), "42")
	c.Check(hc, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "retrieving hardware characteristics for machine \"42\": boom")
}

func (s *serviceSuite) TestRetrieveAvailabilityZone(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().AvailabilityZone(gomock.Any(), "42").
		Return("foo", nil)

	hc, err := NewService(s.state).AvailabilityZone(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hc, gc.DeepEquals, "foo")
}

func (s *serviceSuite) TestSetMachineCloudInstance(c *gc.C) {
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
		hc,
	).Return(nil)

	err := NewService(s.state).SetMachineCloudInstance(context.Background(), "42", "instance-42", "42", hc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetMachineCloudInstanceFails(c *gc.C) {
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
		hc,
	).Return(errors.New("boom"))

	err := NewService(s.state).SetMachineCloudInstance(context.Background(), "42", "instance-42", "42", hc)
	c.Assert(err, gc.ErrorMatches, "setting machine cloud instance for machine \"42\": boom")
}

func (s *serviceSuite) TestDeleteMachineCloudInstance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachineCloudInstance(gomock.Any(), "42").Return(nil)

	err := NewService(s.state).DeleteMachineCloudInstance(context.Background(), "42")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteMachineCloudInstanceFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachineCloudInstance(gomock.Any(), "42").Return(errors.New("boom"))

	err := NewService(s.state).DeleteMachineCloudInstance(context.Background(), "42")
	c.Assert(err, gc.ErrorMatches, "deleting machine cloud instance for machine \"42\": boom")
}

func uintptr(u uint64) *uint64 {
	return &u
}
