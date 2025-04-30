// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation(c *gc.C) *importOperation {
	return &importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "666",
	})
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("666")).Times(1)

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestFailImportMachineWithoutCloudInstance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "0",
	})

	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0")).
		Return("", errors.New("boom"))

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, "importing machine(.*)boom")
}

func (s *importSuite) TestImportMachineWithoutCloudInstance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id: "0",
	})

	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0"))

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestFailImportMachineWithCloudInstance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id: "0",
	})
	cloudInstanceArgs := description.CloudInstanceArgs{
		InstanceId:       "inst-0",
		DisplayName:      "inst-0",
		Architecture:     "amd64",
		Memory:           1024,
		RootDisk:         2048,
		RootDiskSource:   "/",
		CpuCores:         4,
		CpuPower:         16,
		Tags:             []string{"tag0", "tag1"},
		AvailabilityZone: "az-1",
		VirtType:         "vm",
	}
	machine0.SetInstance(cloudInstanceArgs)

	expectedMachineUUID := machine.UUID("deadbeef-1bad-500d-9000-4b1d0d06f00d")
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0")).
		Return(expectedMachineUUID, nil)
	expectedHardwareCharacteristics := &instance.HardwareCharacteristics{
		Arch:             &cloudInstanceArgs.Architecture,
		Mem:              &cloudInstanceArgs.Memory,
		RootDisk:         &cloudInstanceArgs.RootDisk,
		RootDiskSource:   &cloudInstanceArgs.RootDiskSource,
		CpuCores:         &cloudInstanceArgs.CpuCores,
		CpuPower:         &cloudInstanceArgs.CpuPower,
		Tags:             &cloudInstanceArgs.Tags,
		AvailabilityZone: &cloudInstanceArgs.AvailabilityZone,
		VirtType:         &cloudInstanceArgs.VirtType,
	}
	s.service.EXPECT().SetMachineCloudInstance(
		gomock.Any(),
		expectedMachineUUID,
		instance.Id("inst-0"),
		"inst-0",
		expectedHardwareCharacteristics,
	).Return(errors.New("boom"))

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, "importing machine cloud instance(.*)boom")
}

func (s *importSuite) TestImportMachineWithCloudInstance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id: "0",
	})
	cloudInstanceArgs := description.CloudInstanceArgs{
		InstanceId:       "inst-0",
		DisplayName:      "inst-0",
		Architecture:     "amd64",
		Memory:           1024,
		RootDisk:         2048,
		RootDiskSource:   "/",
		CpuCores:         4,
		CpuPower:         16,
		Tags:             []string{"tag0", "tag1"},
		AvailabilityZone: "az-1",
		VirtType:         "vm",
	}
	machine0.SetInstance(cloudInstanceArgs)

	expectedMachineUUID := machine.UUID("deadbeef-1bad-500d-9000-4b1d0d06f00d")
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0")).
		Return(expectedMachineUUID, nil)
	expectedHardwareCharacteristics := &instance.HardwareCharacteristics{
		Arch:             &cloudInstanceArgs.Architecture,
		Mem:              &cloudInstanceArgs.Memory,
		RootDisk:         &cloudInstanceArgs.RootDisk,
		RootDiskSource:   &cloudInstanceArgs.RootDiskSource,
		CpuCores:         &cloudInstanceArgs.CpuCores,
		CpuPower:         &cloudInstanceArgs.CpuPower,
		Tags:             &cloudInstanceArgs.Tags,
		AvailabilityZone: &cloudInstanceArgs.AvailabilityZone,
		VirtType:         &cloudInstanceArgs.VirtType,
	}
	s.service.EXPECT().SetMachineCloudInstance(
		gomock.Any(),
		expectedMachineUUID,
		instance.Id("inst-0"),
		"inst-0",
		expectedHardwareCharacteristics,
	).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}
