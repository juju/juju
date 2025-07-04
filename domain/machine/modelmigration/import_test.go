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
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service: s.service,
		clock:   clock.WallClock,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id:    "666",
		Nonce: "nonce",
	})
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("666"), ptr("nonce")).Return(machine.UUID("uuid"), nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestFailImportMachineWithoutCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id:    "0",
		Nonce: "nonce",
	})

	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0"), ptr("nonce")).Return(machine.UUID(""), errors.New("boom"))

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, "importing machine(.*)boom")
}

func (s *importSuite) TestFailImportMachineWithCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id:    "0",
		Nonce: "nonce",
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
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0"), ptr("nonce")).Return(expectedMachineUUID, nil)
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
		"nonce",
		expectedHardwareCharacteristics,
	).Return(errors.New("boom"))

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, "importing machine cloud instance(.*)boom")
}

func (s *importSuite) TestImportMachineWithCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id:    "0",
		Nonce: "nonce",
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
	s.service.EXPECT().CreateMachine(gomock.Any(), machine.Name("0"), ptr("nonce")).Return(expectedMachineUUID, nil)
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
		"nonce",
		expectedHardwareCharacteristics,
	).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
