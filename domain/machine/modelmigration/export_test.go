// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

var _ = tc.Suite(&exportSuite{})

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *exportSuite) TestFailGetInstanceIDForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	machineNames := []coremachine.Name{"deadbeef"}
	dst.AddMachine(description.MachineArgs{
		Id: string(machineNames[0]),
	})

	machineUUIDs := []coremachine.UUID{"deadbeef-0bad-400d-8000-4b1d0d06f00d"}
	s.service.EXPECT().GetMachineUUID(gomock.Any(), machineNames[0]).
		Return(machineUUIDs[0], nil)
	s.service.EXPECT().InstanceID(gomock.Any(), machineUUIDs[0]).
		Return("", errors.New("boom"))

	op := s.newExportOperation(c)
	err := op.Execute(context.Background(), dst)
	c.Assert(err, tc.ErrorMatches, "retrieving instance ID for machine \"deadbeef\": boom")
}

func (s *exportSuite) TestFailGetHardwareCharacteristicsForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	machineNames := []coremachine.Name{"deadbeef"}
	dst.AddMachine(description.MachineArgs{
		Id: string(machineNames[0]),
	})

	machineUUIDs := []coremachine.UUID{"deadbeef-0bad-400d-8000-4b1d0d06f00d"}
	s.service.EXPECT().GetMachineUUID(gomock.Any(), machineNames[0]).
		Return(machineUUIDs[0], nil)
	s.service.EXPECT().InstanceID(gomock.Any(), machineUUIDs[0]).
		Return("inst-0", nil)
	s.service.EXPECT().HardwareCharacteristics(gomock.Any(), machineUUIDs[0]).
		Return(nil, errors.New("boom"))

	op := s.newExportOperation(c)
	err := op.Execute(context.Background(), dst)
	c.Assert(err, tc.ErrorMatches, "retrieving hardware characteristics for machine \"deadbeef\": boom")
}

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})
	machineNames := []coremachine.Name{"deadbeef"}
	dst.AddMachine(description.MachineArgs{
		Id: string(machineNames[0]),
	})

	machineUUIDs := []coremachine.UUID{"deadbeef-0bad-400d-8000-4b1d0d06f00d"}
	s.service.EXPECT().InstanceID(gomock.Any(), machineUUIDs[0]).
		Return("inst-0", nil)
	s.service.EXPECT().GetMachineUUID(gomock.Any(), machineNames[0]).
		Return(machineUUIDs[0], nil)
	tags := []string{"tag0", "tag1"}
	hc := instance.HardwareCharacteristics{
		Arch:             ptr("amd64"),
		Mem:              ptr(uint64(1024)),
		RootDisk:         ptr(uint64(2048)),
		RootDiskSource:   ptr("/"),
		CpuCores:         ptr(uint64(4)),
		CpuPower:         ptr(uint64(16)),
		Tags:             &tags,
		AvailabilityZone: ptr("az-1"),
		VirtType:         ptr("vm"),
	}
	s.service.EXPECT().HardwareCharacteristics(gomock.Any(), machineUUIDs[0]).
		Return(&hc, nil)

	op := s.newExportOperation(c)
	err := op.Execute(context.Background(), dst)
	c.Assert(err, jc.ErrorIsNil)

	actualMachines := dst.Machines()
	c.Check(len(actualMachines), tc.Equals, 1)
	c.Check(actualMachines[0].Id(), tc.Equals, machineNames[0].String())

	cloudInstance := actualMachines[0].Instance()
	c.Check(cloudInstance.Architecture(), tc.Equals, "amd64")
	c.Check(cloudInstance.Memory(), tc.Equals, uint64(1024))
	c.Check(cloudInstance.RootDisk(), tc.Equals, uint64(2048))
	c.Check(cloudInstance.RootDiskSource(), tc.Equals, "/")
	c.Check(cloudInstance.CpuCores(), tc.Equals, uint64(4))
	c.Check(cloudInstance.CpuPower(), tc.Equals, uint64(16))
	c.Check(cloudInstance.Tags(), jc.SameContents, tags)
	c.Check(cloudInstance.AvailabilityZone(), tc.Equals, "az-1")
	c.Check(cloudInstance.VirtType(), tc.Equals, "vm")
}

func ptr[T any](u T) *T {
	return &u
}
