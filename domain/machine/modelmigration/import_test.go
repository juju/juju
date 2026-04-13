// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
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
	m0 := model.AddMachine(description.MachineArgs{
		Id:        "666",
		Nonce:     "nonce",
		Base:      base.MakeDefaultBase("ubuntu", "24.04").String(),
		Placement: "0",
		Hostname:  "host-name-123",
	})
	m0.SetConstraints(description.ConstraintsArgs{
		CpuCores: 8,
		Memory:   1024,
	})

	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("666"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		},
		constraints.Constraints{
			CpuCores: new(uint64(8)),
			Mem:      new(uint64(1024)),
		},
	).Return(machine.UUID("uuid"), nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestFailImportMachineWithoutCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	model.AddMachine(description.MachineArgs{
		Id:       "0",
		Nonce:    "nonce",
		Base:     base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname: "host-name-123",
	})

	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("0"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{},
		constraints.Constraints{},
	).Return(machine.UUID(""), errors.New("boom"))

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorMatches, "importing machine(.*)boom")
}

func (s *importSuite) TestFailImportMachineWithCloudInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id:       "0",
		Nonce:    "nonce",
		Base:     base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname: "host-name-123",
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

	expectedMachineUUID := tc.Must(c, machine.NewUUID)
	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("0"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{},
		constraints.Constraints{},
	).Return(expectedMachineUUID, nil)
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
		Id:       "0",
		Nonce:    "nonce",
		Base:     base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname: "host-name-123",
	})
	cloudInstanceArgs := description.CloudInstanceArgs{
		InstanceId:       "inst-0",
		DisplayName:      "inst-0",
		Architecture:     "arm64",
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

	expectedMachineUUID := tc.Must(c, machine.NewUUID)
	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("0"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.ARM64,
		},
		deployment.Placement{},
		constraints.Constraints{},
	).Return(expectedMachineUUID, nil)
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

func (s *importSuite) TestImportMachineWithContainers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id:       "666",
		Nonce:    "nonce",
		Base:     base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname: "host-name-123",
	})
	machine0.AddContainer(description.MachineArgs{
		Id:        "666/lxd/0",
		Nonce:     "nonce",
		Placement: "lxd:666",
		Base:      base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname:  "host-name-container-456",
	})
	machine0.AddContainer(description.MachineArgs{
		Id:        "666/lxd/1",
		Nonce:     "nonce",
		Placement: "lxd:666",
		Base:      base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname:  "host-name-container-789",
	})

	expectedMachineUUID := tc.Must(c, machine.NewUUID)

	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("666"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{},
		constraints.Constraints{},
	).Return(expectedMachineUUID, nil)

	s.service.EXPECT().CreateSubordinateMachine(
		gomock.Any(),
		"host-name-container-456",
		machine.Name("666/lxd/0"),
		expectedMachineUUID,
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "666",
		},
		constraints.Constraints{},
	).Return(machine.UUID("container-uuid-0"), nil)

	s.service.EXPECT().CreateSubordinateMachine(
		gomock.Any(),
		"host-name-container-789",
		machine.Name("666/lxd/1"),
		expectedMachineUUID,
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.AMD64,
		},
		deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "666",
		},
		constraints.Constraints{},
	).Return(machine.UUID("container-uuid-1"), nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportMachineWithContainerWithCloudInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	machine0 := model.AddMachine(description.MachineArgs{
		Id:       "0",
		Nonce:    "nonce",
		Base:     base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname: "host-name-123",
	})
	container0 := machine0.AddContainer(description.MachineArgs{
		Id:        "0/lxd/0",
		Nonce:     "nonce",
		Placement: "lxd:0",
		Base:      base.MakeDefaultBase("ubuntu", "24.04").String(),
		Hostname:  "host-name-container-456",
	})

	cloudInstanceArgs := description.CloudInstanceArgs{
		InstanceId:       "inst-0",
		DisplayName:      "inst-0",
		Architecture:     "arm64",
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
	container0.SetInstance(cloudInstanceArgs)

	expectedMachineUUID := tc.Must(c, machine.NewUUID)
	expectedContainerUUID := tc.Must(c, machine.NewUUID)

	s.service.EXPECT().CreateMachine(
		gomock.Any(),
		"host-name-123",
		machine.Name("0"),
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.ARM64,
		},
		deployment.Placement{},
		constraints.Constraints{},
	).Return(expectedMachineUUID, nil)
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

	s.service.EXPECT().CreateSubordinateMachine(
		gomock.Any(),
		"host-name-container-456",
		machine.Name("0/lxd/0"),
		expectedMachineUUID,
		new("nonce"),
		deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04/stable",
			Architecture: architecture.ARM64,
		},
		deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "0",
		},
		constraints.Constraints{},
	).Return(expectedContainerUUID, nil)
	s.service.EXPECT().SetMachineCloudInstance(
		gomock.Any(),
		expectedContainerUUID,
		instance.Id("inst-0"),
		"inst-0",
		"nonce",
		expectedHardwareCharacteristics,
	).Return(nil)

	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)
}
