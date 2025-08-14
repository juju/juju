// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)

	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		service: s.service,
		clock:   clock.WallClock,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *exportSuite) TestFailGetHardwareCharacteristicsForExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	machineNames := []coremachine.Name{"deadbeef"}
	machineUUIDs := []coremachine.UUID{"deadbeef-0bad-400d-8000-4b1d0d06f00d"}

	dst.AddMachine(description.MachineArgs{
		Id: string(machineNames[0]),
	})

	s.service.EXPECT().GetMachines(gomock.Any()).Return([]machine.ExportMachine{
		{
			Name:       machineNames[0],
			UUID:       machineUUIDs[0],
			InstanceID: "inst-0",
		},
	}, nil)
	s.service.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUIDs[0]).
		Return(nil, errors.New("boom"))

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorMatches, ".*retrieving hardware characteristics for machine \"deadbeef\": boom")
}

func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	machineName := coremachine.Name("0")
	machineUUID := coremachine.GenUUID(c)

	s.service.EXPECT().GetMachines(gomock.Any()).Return([]machine.ExportMachine{
		{
			Name:         machineName,
			UUID:         machineUUID,
			Nonce:        "a nonce",
			PasswordHash: "shhhh!",
			Placement:    "place it here",
			Base:         "ubuntu@24.04/stable",
			InstanceID:   "inst-0",
		},
	}, nil)
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
	s.service.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUID).
		Return(&hc, nil)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	actualMachines := dst.Machines()
	c.Assert(actualMachines, tc.HasLen, 1)

	c.Check(actualMachines[0].Id(), tc.Equals, machineName.String())
	c.Check(actualMachines[0].Nonce(), tc.Equals, "a nonce")
	c.Check(actualMachines[0].PasswordHash(), tc.Equals, "shhhh!")
	c.Check(actualMachines[0].Placement(), tc.Equals, "place it here")
	c.Check(actualMachines[0].Base(), tc.Equals, "ubuntu@24.04/stable")

	cloudInstance := actualMachines[0].Instance()
	c.Check(cloudInstance.Architecture(), tc.Equals, "amd64")
	c.Check(cloudInstance.Memory(), tc.Equals, uint64(1024))
	c.Check(cloudInstance.RootDisk(), tc.Equals, uint64(2048))
	c.Check(cloudInstance.RootDiskSource(), tc.Equals, "/")
	c.Check(cloudInstance.CpuCores(), tc.Equals, uint64(4))
	c.Check(cloudInstance.CpuPower(), tc.Equals, uint64(16))
	c.Check(cloudInstance.Tags(), tc.SameContents, tags)
	c.Check(cloudInstance.AvailabilityZone(), tc.Equals, "az-1")
	c.Check(cloudInstance.VirtType(), tc.Equals, "vm")
}

func (s *exportSuite) TestExportContainer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	machineName := coremachine.Name("0")
	machineUUID := coremachine.GenUUID(c)
	containerName := coremachine.Name("0/lxd/0")
	containerUUID := coremachine.GenUUID(c)

	// NOTE: We return the container machine first, to check the export code can
	// handle this.
	s.service.EXPECT().GetMachines(gomock.Any()).Return([]machine.ExportMachine{
		{
			Name:         containerName,
			UUID:         containerUUID,
			Nonce:        "another nonce",
			PasswordHash: "shhhh!",
			Placement:    "place it there",
			Base:         "ubuntu@24.04/stable",
			InstanceID:   "inst-0",
		},
		{
			Name:         machineName,
			UUID:         machineUUID,
			Nonce:        "a nonce",
			PasswordHash: "shhhh!",
			Placement:    "place it here",
			Base:         "ubuntu@24.04/stable",
			InstanceID:   "inst-0",
		},
	}, nil)

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
	s.service.EXPECT().GetHardwareCharacteristics(gomock.Any(), machineUUID).
		Return(&hc, nil)
	s.service.EXPECT().GetHardwareCharacteristics(gomock.Any(), containerUUID).
		Return(&hc, nil)

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	actualMachines := dst.Machines()
	c.Assert(actualMachines, tc.HasLen, 1)

	c.Check(actualMachines[0].Id(), tc.Equals, machineName.String())
	c.Check(actualMachines[0].Nonce(), tc.Equals, "a nonce")
	c.Check(actualMachines[0].PasswordHash(), tc.Equals, "shhhh!")
	c.Check(actualMachines[0].Placement(), tc.Equals, "place it here")
	c.Check(actualMachines[0].Base(), tc.Equals, "ubuntu@24.04/stable")

	actualContainers := actualMachines[0].Containers()
	c.Assert(actualContainers, tc.HasLen, 1)

	c.Check(actualContainers[0].Id(), tc.Equals, containerName.String())
	c.Check(actualContainers[0].Nonce(), tc.Equals, "another nonce")
	c.Check(actualContainers[0].PasswordHash(), tc.Equals, "shhhh!")
	c.Check(actualContainers[0].Placement(), tc.Equals, "place it there")
	c.Check(actualContainers[0].Base(), tc.Equals, "ubuntu@24.04/stable")
}
